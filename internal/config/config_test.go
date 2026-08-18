package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`
registry_prefix: docker.io/myorg
repos:
  - hello:latest
`), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.PollInterval != 5*time.Minute {
		t.Errorf("default poll_interval = %s", c.PollInterval)
	}
	if c.ArtifactsDir == "" || c.StateFile == "" || c.LockFile == "" {
		t.Errorf("defaults not applied: %+v", c)
	}
}

func TestLoad_MissingRegistryPrefix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	_ = os.WriteFile(path, []byte(`repos: ["x:latest"]`), 0o644)
	if _, err := Load(path); err == nil {
		t.Fatal("want error for missing registry_prefix")
	}
}

func TestLoad_EmptyRepos(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	_ = os.WriteFile(path, []byte(`registry_prefix: docker.io/myorg`), 0o644)
	if _, err := Load(path); err == nil {
		t.Fatal("want error for empty repos")
	}
}

func TestParsedRepos(t *testing.T) {
	c := &Config{
		RegistryPrefix: "docker.io/myorg",
		Repos:          []string{"foo:1.2.3", "bar", " baz:latest "},
	}
	got := c.ParsedRepos()
	if len(got) != 3 {
		t.Fatalf("got %d, want 3: %+v", len(got), got)
	}
	if got[0].FullRef != "docker.io/myorg/foo:1.2.3" {
		t.Errorf("[0] = %+v", got[0])
	}
	if got[1].Tag != "latest" {
		t.Errorf("bare name should default to :latest, got tag=%q", got[1].Tag)
	}
	if got[2].Name != "baz" {
		t.Errorf("[2] name = %q after trim", got[2].Name)
	}
}

func TestAllowedTargetRootsValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Relative root -> error.
	_ = os.WriteFile(path, []byte(`
registry_prefix: docker.io/myorg
repos: ["a:latest"]
allowed_target_roots: ["opt/apps"]
`), 0o644)
	if _, err := Load(path); err == nil {
		t.Fatal("want error for relative allowed_target_roots entry")
	}

	// "/" as a root -> error (defeats the allowlist).
	_ = os.WriteFile(path, []byte(`
registry_prefix: docker.io/myorg
repos: ["a:latest"]
allowed_target_roots: ["/"]
`), 0o644)
	if _, err := Load(path); err == nil {
		t.Fatal("want error for / as allowed_target_roots entry")
	}

	// Valid roots are cleaned (trailing slash dropped).
	_ = os.WriteFile(path, []byte(`
registry_prefix: docker.io/myorg
repos: ["a:latest"]
allowed_target_roots: ["/var/www/html/", "/opt/tomcat/instances"]
`), 0o644)
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.AllowedTargetRoots[0] != "/var/www/html" {
		t.Errorf("root[0] = %q, want cleaned /var/www/html", c.AllowedTargetRoots[0])
	}
}

func TestEnvExpansion(t *testing.T) {
	t.Setenv("FOO_TEST_TOKEN", "secret-value")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	_ = os.WriteFile(path, []byte(`
registry_prefix: docker.io/myorg
repos: ["a:latest"]
auth:
  token: $FOO_TEST_TOKEN
`), 0o644)

	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Auth.Token != "secret-value" {
		t.Errorf("Token = %q, want expanded value", c.Auth.Token)
	}
}
