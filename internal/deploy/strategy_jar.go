package deploy

import (
	"context"
	"path/filepath"
)

type jarStrategy struct{}

func (jarStrategy) Apply(ctx context.Context, req Request) error {
	mgr := req.Spec.EffectiveServiceManager()
	src := filepath.Join(req.ArtifactDir, req.Spec.SourceFile)

	req.Logger.Info("stopping service", "unit", req.Spec.ServiceName, "manager", mgr)
	if err := stopService(ctx, mgr, req.Spec.ServiceName); err != nil {
		return err
	}

	req.Logger.Info("copying jar", "src", src, "dst", req.Spec.TargetPath)
	if err := copyFile(src, req.Spec.TargetPath, req.Spec.TargetMode, req.Spec.TargetOwner); err != nil {
		return err
	}

	req.Logger.Info("starting service", "unit", req.Spec.ServiceName)
	return startService(ctx, mgr, req.Spec.ServiceName)
}
