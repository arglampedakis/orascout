package deploy

import (
	"context"
	"fmt"
)

type jarStrategy struct{}

func (jarStrategy) Apply(ctx context.Context, req Request) error {
	mgr := req.Spec.EffectiveServiceManager()
	src, err := securePathJoin(req.ArtifactDir, req.Spec.SourceFile)
	if err != nil {
		return fmt.Errorf("source.file rejected: %w", err)
	}

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
