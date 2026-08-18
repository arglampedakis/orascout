// Package registry wraps oras-go to expose only the small surface orascout needs:
// resolving manifest digests, fetching manifest annotations, pulling artifacts
// into a directory, and pushing a single file back (for log shipping).
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

// Auth carries the credentials used for all repositories.
type Auth struct {
	Username string
	Password string
	Token    string
}

// Client is a thin wrapper around oras-go.
type Client struct {
	auth     Auth
	insecure bool
}

// New constructs a Client.
func New(a Auth, insecure bool) *Client {
	return &Client{auth: a, insecure: insecure}
}

// ResolveDigest returns the manifest digest currently associated with ref.
// ref must be a fully-qualified reference: "host/repo:tag".
func (c *Client) ResolveDigest(ctx context.Context, ref string) (string, error) {
	repo, tag, err := c.repoAndTag(ref)
	if err != nil {
		return "", err
	}
	desc, err := repo.Resolve(ctx, tag)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", ref, err)
	}
	return string(desc.Digest), nil
}

// FetchAnnotations returns the manifest's annotation map for ref. It works for
// both image-manifest and image-index media types: in either case the parsed
// manifest's top-level Annotations field is returned.
func (c *Client) FetchAnnotations(ctx context.Context, ref string) (map[string]string, error) {
	repo, tag, err := c.repoAndTag(ref)
	if err != nil {
		return nil, err
	}
	desc, rc, err := repo.FetchReference(ctx, tag)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", ref, err)
	}
	defer rc.Close()

	body, err := io.ReadAll(io.LimitReader(rc, 4<<20)) // 4 MiB cap on manifest size
	if err != nil {
		return nil, fmt.Errorf("read manifest body: %w", err)
	}

	// Both Manifest and Index carry a top-level Annotations map; a partial
	// decode against a shared shape is enough.
	var m struct {
		Annotations map[string]string `json:"annotations"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("decode manifest (digest=%s, mediaType=%s): %w", desc.Digest, desc.MediaType, err)
	}
	if m.Annotations == nil {
		m.Annotations = map[string]string{}
	}
	return m.Annotations, nil
}

// Pull copies all blobs referenced by ref into destDir. destDir is created if
// it does not exist. Any existing contents are not cleared (caller's choice).
func (c *Client) Pull(ctx context.Context, ref, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("mkdir destDir: %w", err)
	}
	repo, tag, err := c.repoAndTag(ref)
	if err != nil {
		return err
	}
	store, err := file.New(destDir)
	if err != nil {
		return fmt.Errorf("file store: %w", err)
	}
	defer store.Close()

	if _, err := oras.Copy(ctx, repo, tag, store, tag, oras.DefaultCopyOptions); err != nil {
		return fmt.Errorf("oras copy %s -> %s: %w", ref, destDir, err)
	}
	return nil
}

// PushFile pushes a single local file to ref. mediaType is the layer media
// type (e.g. "text/plain" for log files). The artifact type on the manifest
// is set to ARTIFACT_TYPE_LOGS to keep these separate from real deployments.
func (c *Client) PushFile(ctx context.Context, ref, localPath, mediaType string) error {
	repo, tag, err := c.repoAndTag(ref)
	if err != nil {
		return err
	}

	// oras-go's file.Store wants files placed under its workDir to add by
	// relative path. Use the file's own directory as the workDir.
	workDir := filepath.Dir(localPath)
	store, err := file.New(workDir)
	if err != nil {
		return fmt.Errorf("file store: %w", err)
	}
	defer store.Close()

	layerName := filepath.Base(localPath)
	layerDesc, err := store.Add(ctx, layerName, mediaType, "")
	if err != nil {
		return fmt.Errorf("add layer %s: %w", localPath, err)
	}

	manifestDesc, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, "application/vnd.dev.orascout.logs.v1+json",
		oras.PackManifestOptions{
			Layers: []ocispec.Descriptor{layerDesc},
		},
	)
	if err != nil {
		return fmt.Errorf("pack manifest: %w", err)
	}
	if err := store.Tag(ctx, manifestDesc, tag); err != nil {
		return fmt.Errorf("tag manifest: %w", err)
	}

	if _, err := oras.Copy(ctx, store, tag, repo, tag, oras.DefaultCopyOptions); err != nil {
		return fmt.Errorf("copy to remote: %w", err)
	}
	return nil
}

// repoAndTag parses ref into an authenticated *remote.Repository and a tag.
func (c *Client) repoAndTag(ref string) (*remote.Repository, string, error) {
	host, path, tag, err := splitRef(ref)
	if err != nil {
		return nil, "", err
	}
	repo, err := remote.NewRepository(host + "/" + path)
	if err != nil {
		return nil, "", fmt.Errorf("new repository %s: %w", ref, err)
	}
	repo.PlainHTTP = c.insecure
	repo.Client = c.authClient(host)
	return repo, tag, nil
}

func (c *Client) authClient(host string) *auth.Client {
	cred := auth.Credential{
		Username:    c.auth.Username,
		Password:    c.auth.Password,
		AccessToken: c.auth.Token,
	}
	cli := &auth.Client{
		Client: retry.DefaultClient,
		Cache:  auth.NewCache(),
		Credential: func(ctx context.Context, registry string) (auth.Credential, error) {
			if cred.Username == "" && cred.Password == "" && cred.AccessToken == "" {
				return auth.EmptyCredential, nil
			}
			return cred, nil
		},
	}
	cli.SetUserAgent("orascout/0.1")
	return cli
}

// splitRef parses "host/path/to/repo:tag" into its three parts.
func splitRef(ref string) (host, path, tag string, err error) {
	r := strings.TrimSpace(ref)
	if r == "" {
		return "", "", "", fmt.Errorf("empty ref")
	}
	at, sep := r, strings.LastIndex(r, ":")
	// Guard against ports in the host (host:5000/...).
	if sep > strings.Index(r, "/") {
		tag = r[sep+1:]
		at = r[:sep]
	} else {
		tag = "latest"
	}
	slash := strings.Index(at, "/")
	if slash < 0 {
		return "", "", "", fmt.Errorf("ref %q must contain host/repo", ref)
	}
	return at[:slash], at[slash+1:], tag, nil
}
