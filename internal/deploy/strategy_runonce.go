package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
)

type runOnceStrategy struct{}

func (runOnceStrategy) Apply(ctx context.Context, req Request) error {
	file, err := securePathJoin(req.ArtifactDir, req.Spec.SourceFile)
	if err != nil {
		return fmt.Errorf("source.file rejected: %w", err)
	}

	tokens, err := parseShellCommand(req.Spec.RunonceCommand)
	if err != nil {
		return fmt.Errorf("parse command: %w", err)
	}
	if len(tokens) == 0 {
		return fmt.Errorf("empty runonce command")
	}
	tokens = applyTemplate(tokens, file)

	req.Logger.Info("running one-shot", "argv", tokens)
	cmd := exec.CommandContext(ctx, tokens[0], tokens[1:]...)
	cmd.Dir = req.ArtifactDir
	cmd.Stdout = logWriter{req.Logger, slog.LevelInfo, "runonce"}
	cmd.Stderr = logWriter{req.Logger, slog.LevelWarn, "runonce"}
	return cmd.Run()
}
