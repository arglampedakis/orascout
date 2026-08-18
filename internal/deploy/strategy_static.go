package deploy

import (
	"context"
	"path/filepath"
)

type staticStrategy struct{}

func (staticStrategy) Apply(ctx context.Context, req Request) error {
	src := filepath.Join(req.ArtifactDir, req.Spec.SourceDir)

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
