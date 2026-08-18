// Package deploy holds the deploy-strategy interface, the dispatcher that
// selects a strategy from a parsed annotations.Spec, hook execution, and
// post-deploy healthcheck.
package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/arglampedakis/orascout/pkg/annotations"
)

// Request is everything a strategy needs to do its job.
type Request struct {
	// ArtifactDir is the absolute path the artifact was pulled into.
	ArtifactDir string
	// Repo is the "repo:tag" key for logging / hook env.
	Repo string
	// Digest is the new manifest digest.
	Digest string
	// PrevDigest is the previous deployed digest, or "" on first deploy.
	PrevDigest string
	// Spec is the parsed manifest annotations.
	Spec *annotations.Spec
	// Logger gets routed to the daemon's structured logger.
	Logger *slog.Logger
}

// Strategy is a deploy action selected by annotations.Type.
type Strategy interface {
	Apply(ctx context.Context, req Request) error
}

// Dispatch returns the Strategy that should handle req.Spec.Type.
func Dispatch(t annotations.Type) (Strategy, error) {
	switch t {
	case annotations.TypeJar:
		return jarStrategy{}, nil
	case annotations.TypeWar:
		return warStrategy{}, nil
	case annotations.TypeStatic:
		return staticStrategy{}, nil
	case annotations.TypeRunOnce:
		return runOnceStrategy{}, nil
	case annotations.TypeHookOnly:
		return hookOnlyStrategy{}, nil
	}
	return nil, fmt.Errorf("no strategy for type %q", t)
}

// Run executes the full deploy: pre-hook → strategy → healthcheck → post-hook.
// Errors before/including the strategy and a failed healthcheck abort the
// deploy. A failed post-hook is logged as a warning only.
func Run(ctx context.Context, req Request) error {
	if err := runHook(ctx, req, "pre", req.Spec.HookPre); err != nil {
		return fmt.Errorf("pre-hook: %w", err)
	}

	strat, err := Dispatch(req.Spec.Type)
	if err != nil {
		return err
	}
	if err := strat.Apply(ctx, req); err != nil {
		return fmt.Errorf("strategy %s: %w", req.Spec.Type, err)
	}

	if req.Spec.HealthCmd != "" {
		if err := healthcheck(ctx, req); err != nil {
			return fmt.Errorf("healthcheck: %w", err)
		}
	}

	if err := runHook(ctx, req, "post", req.Spec.HookPost); err != nil {
		req.Logger.Warn("post-hook failed (deploy still considered successful)", "err", err)
	}
	return nil
}

// runHook executes a hook script at hookRelPath (relative to the artifact dir).
// A blank path is a no-op.
func runHook(ctx context.Context, req Request, phase, hookRelPath string) error {
	if hookRelPath == "" {
		return nil
	}
	abs := filepath.Join(req.ArtifactDir, hookRelPath)
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("hook script %s not found in artifact: %w", hookRelPath, err)
	}

	req.Logger.Info("running hook", "phase", phase, "script", hookRelPath)
	cmd := exec.CommandContext(ctx, abs)
	cmd.Dir = req.ArtifactDir
	cmd.Env = append(os.Environ(),
		"ORASCOUT_ARTIFACT_DIR="+req.ArtifactDir,
		"ORASCOUT_REPO="+req.Repo,
		"ORASCOUT_DIGEST="+req.Digest,
		"ORASCOUT_PREV_DIGEST="+req.PrevDigest,
		"ORASCOUT_TYPE="+string(req.Spec.Type),
		"ORASCOUT_PHASE="+phase,
	)
	cmd.Stdout = logWriter{req.Logger, slog.LevelInfo, "hook." + phase}
	cmd.Stderr = logWriter{req.Logger, slog.LevelWarn, "hook." + phase}
	return cmd.Run()
}

// healthcheck runs req.Spec.HealthCmd via /bin/sh -c until it exits 0 or the
// timeout elapses. Each attempt is separated by req.Spec.HealthInterval.
func healthcheck(ctx context.Context, req Request) error {
	deadline := time.Now().Add(req.Spec.HealthTimeout)
	req.Logger.Info("running healthcheck", "cmd", req.Spec.HealthCmd, "timeout", req.Spec.HealthTimeout)
	for {
		attemptCtx, cancel := context.WithTimeout(ctx, req.Spec.HealthInterval+5*time.Second)
		cmd := exec.CommandContext(attemptCtx, "/bin/sh", "-c", req.Spec.HealthCmd)
		out, err := cmd.CombinedOutput()
		cancel()
		if err == nil {
			req.Logger.Info("healthcheck passed")
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("healthcheck did not pass within %s: %w (last output: %s)", req.Spec.HealthTimeout, err, strings.TrimSpace(string(out)))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(req.Spec.HealthInterval):
		}
	}
}

// logWriter adapts an io.Writer so subprocess stdout/stderr lines flow into slog.
type logWriter struct {
	logger *slog.Logger
	level  slog.Level
	source string
}

func (w logWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if line == "" {
			continue
		}
		w.logger.Log(context.Background(), w.level, line, "source", w.source)
	}
	return len(p), nil
}

