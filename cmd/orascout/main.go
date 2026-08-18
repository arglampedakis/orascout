// Command orascout is a daemon that watches an OCI registry for ORAS artifact
// updates and deploys them based on manifest annotations. See ../../SPEC.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/arglampedakis/orascout/internal/config"
	"github.com/arglampedakis/orascout/internal/watcher"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "run":
		os.Exit(cmdRun(os.Args[2:], false))
	case "check":
		os.Exit(cmdRun(os.Args[2:], true))
	case "version", "-v", "--version":
		fmt.Println("orascout", version)
		os.Exit(0)
	case "help", "-h", "--help":
		usage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `orascout %s - watch an OCI registry for ORAS artifact updates and deploy them.

Usage:
  orascout run    -c <config.yaml> [--once]   start the daemon (or one cycle then exit)
  orascout check  -c <config.yaml>            dry-run: report what would deploy
  orascout version
  orascout help

Configuration: see examples/config.yaml.
Annotation schema: see SPEC.md.
`, version)
}

func cmdRun(args []string, dryRun bool) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	cfgPath := fs.String("c", "/etc/orascout/config.yaml", "path to config.yaml")
	once := fs.Bool("once", false, "run one cycle and exit (ignored by `check`)")
	logLevel := fs.String("log-level", "info", "debug|info|warn|error")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}

	logger, closeLog, err := buildLogger(cfg.LogFile, *logLevel)
	if err != nil {
		fmt.Fprintln(os.Stderr, "logger:", err)
		return 1
	}
	defer closeLog()

	w, err := watcher.New(cfg, logger)
	if err != nil {
		logger.Error("watcher init", "err", err)
		return 1
	}
	w.DryRun = dryRun

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if dryRun || *once {
		if err := w.RunOnce(ctx); err != nil {
			logger.Error("run-once failed", "err", err)
			return 1
		}
		return 0
	}

	if err := w.Run(ctx); err != nil && err != context.Canceled {
		logger.Error("daemon exited with error", "err", err)
		return 1
	}
	logger.Info("orascout stopped cleanly")
	return 0
}

// buildLogger constructs an slog.Logger that writes to stderr and, if logFile
// is set, also to that file (best-effort, created if absent).
func buildLogger(logFile, level string) (*slog.Logger, func(), error) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return nil, nil, fmt.Errorf("log-level: %w", err)
	}

	var w io.Writer = os.Stderr
	closer := func() {}
	if logFile != "" {
		if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
			return nil, nil, fmt.Errorf("mkdir log dir: %w", err)
		}
		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, nil, fmt.Errorf("open log file: %w", err)
		}
		w = io.MultiWriter(os.Stderr, f)
		closer = func() { _ = f.Close() }
	}

	handler := slog.NewTextHandler(w, &slog.HandlerOptions{Level: lvl})
	return slog.New(handler), closer, nil
}
