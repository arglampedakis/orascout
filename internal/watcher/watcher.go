// Package watcher implements the main poll loop: for each configured repo,
// resolve the remote manifest digest, compare to the on-disk state, and on a
// change pull the artifact and dispatch it to a deploy strategy.
package watcher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/arglampedakis/orascout/internal/config"
	"github.com/arglampedakis/orascout/internal/deploy"
	"github.com/arglampedakis/orascout/internal/lockfile"
	"github.com/arglampedakis/orascout/internal/registry"
	"github.com/arglampedakis/orascout/internal/state"
	"github.com/arglampedakis/orascout/pkg/annotations"
)

// Watcher polls a registry on a fixed interval.
type Watcher struct {
	cfg      *config.Config
	registry *registry.Client
	state    *state.Store
	logger   *slog.Logger

	// DryRun, when true, resolves and reports what would deploy but skips
	// the pull/deploy/state-update.
	DryRun bool
}

// New builds a Watcher from a parsed config.
func New(cfg *config.Config, logger *slog.Logger) (*Watcher, error) {
	st, err := state.Open(cfg.StateFile)
	if err != nil {
		return nil, err
	}
	if len(cfg.AllowedTargetRoots) == 0 {
		logger.Warn("allowed_target_roots is not set — deploys may write anywhere outside the built-in denylist of system paths; set it in config.yaml to restrict deploys to specific directories")
	}
	return &Watcher{
		cfg: cfg,
		registry: registry.New(registry.Auth{
			Username: cfg.Auth.Username,
			Password: cfg.Auth.Password,
			Token:    cfg.Auth.Token,
		}, cfg.Insecure),
		state:  st,
		logger: logger,
	}, nil
}

// Run loops forever (or until ctx is cancelled), invoking RunOnce on each tick.
// The first cycle fires immediately on entry.
func (w *Watcher) Run(ctx context.Context) error {
	w.logger.Info("orascout starting",
		"poll_interval", w.cfg.PollInterval,
		"repos", len(w.cfg.ParsedRepos()),
		"dry_run", w.DryRun,
	)

	if err := w.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Error("cycle ended with error", "err", err)
	}

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				w.logger.Error("cycle ended with error", "err", err)
			}
		}
	}
}

// RunOnce executes a single full cycle: take lock, iterate repos, release lock.
// A lock contention is logged at info and returns nil (this is normal).
func (w *Watcher) RunOnce(ctx context.Context) error {
	lock, err := lockfile.Acquire(w.cfg.LockFile)
	if errors.Is(err, lockfile.ErrLocked) {
		w.logger.Info("another orascout cycle is running; skipping")
		return nil
	}
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer func() { _ = lock.Release() }()

	deployed := 0
	for _, ref := range w.cfg.ParsedRepos() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		did, err := w.cycleOne(ctx, ref)
		if err != nil {
			w.logger.Error("repo cycle failed", "ref", ref.FullRef, "err", err)
			continue
		}
		if did {
			deployed++
		}
	}

	if deployed > 0 && w.cfg.LogsPush != "" {
		w.pushLogs(ctx)
	}
	return nil
}

// cycleOne handles one repo. Returns (deployed, err). A returned err is
// already-logged context; the caller continues to the next repo.
func (w *Watcher) cycleOne(ctx context.Context, ref config.RepoRef) (bool, error) {
	logger := w.logger.With("ref", ref.FullRef)

	currentDigest, err := w.registry.ResolveDigest(ctx, ref.FullRef)
	if err != nil {
		return false, fmt.Errorf("resolve digest: %w", err)
	}

	prev, _ := w.state.Get(ref.FullRef)
	if currentDigest == prev.Digest {
		logger.Debug("no change", "digest", currentDigest)
		return false, nil
	}

	if prev.Digest == "" {
		logger.Info("new artifact (first deploy)", "digest", currentDigest)
	} else {
		logger.Info("artifact changed", "old", prev.Digest, "new", currentDigest)
	}

	rawAnnotations, err := w.registry.FetchAnnotations(ctx, ref.FullRef)
	if err != nil {
		return false, fmt.Errorf("fetch annotations: %w", err)
	}
	spec, err := annotations.Parse(rawAnnotations)
	if err != nil {
		return false, fmt.Errorf("invalid annotations: %w", err)
	}

	if w.DryRun {
		logger.Info("dry-run: would deploy",
			"type", spec.Type,
			"service", spec.ServiceName,
			"target", spec.TargetPath,
		)
		return false, nil
	}

	artifactDir := filepath.Join(w.cfg.ArtifactsDir, ref.Name)
	// Clear previous pull contents so stale files don't linger.
	if err := os.RemoveAll(artifactDir); err != nil {
		return false, fmt.Errorf("clean artifact dir: %w", err)
	}
	if err := w.registry.Pull(ctx, ref.FullRef, artifactDir); err != nil {
		return false, fmt.Errorf("pull: %w", err)
	}

	req := deploy.Request{
		ArtifactDir:  artifactDir,
		Repo:         ref.FullRef,
		Digest:       currentDigest,
		PrevDigest:   prev.Digest,
		Spec:         spec,
		AllowedRoots: w.cfg.AllowedTargetRoots,
		Logger:       logger,
	}
	if err := deploy.Run(ctx, req); err != nil {
		return false, fmt.Errorf("deploy: %w", err)
	}

	if err := w.state.Set(ref.FullRef, state.Entry{
		Digest:     currentDigest,
		DeployedAt: time.Now().UTC(),
		Type:       string(spec.Type),
	}); err != nil {
		return false, fmt.Errorf("persist state: %w", err)
	}
	logger.Info("deployed", "digest", currentDigest, "type", spec.Type)
	return true, nil
}

// pushLogs ships the daemon log file to the configured logs-push target.
func (w *Watcher) pushLogs(ctx context.Context) {
	if w.cfg.LogFile == "" {
		w.logger.Debug("logs_push set but no log_file configured; skipping push")
		return
	}
	target := strings.TrimSuffix(w.cfg.RegistryPrefix, "/") + "/" + w.cfg.LogsPush
	if !strings.Contains(w.cfg.LogsPush, ":") {
		target += ":latest"
	}
	w.logger.Info("pushing log file", "target", target, "file", w.cfg.LogFile)
	if err := w.registry.PushFile(ctx, target, w.cfg.LogFile, "text/plain"); err != nil {
		w.logger.Warn("log push failed", "err", err)
	}
}
