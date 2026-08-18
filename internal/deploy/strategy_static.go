package deploy

import (
	"context"
	"fmt"
)

type staticStrategy struct{}

func (staticStrategy) Apply(ctx context.Context, req Request) error {
	src, err := securePathJoin(req.ArtifactDir, req.Spec.SourceDir)
	if err != nil {
		return fmt.Errorf("source.dir rejected: %w", err)
	}

	clear := true // default per spec §3.3
	if req.Spec.TargetClear != nil {
		clear = *req.Spec.TargetClear
	}

	req.Logger.Info("syncing static directory",
		"src", src,
		"dst", req.Spec.TargetPath,
		"clear", clear,
	)
	return copyDir(src, req.Spec.TargetPath, clear, req.Spec.TargetMode, req.Spec.TargetOwner)
}
