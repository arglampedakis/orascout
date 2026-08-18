package deploy

import (
	"context"
	"path/filepath"
)

type warStrategy struct{}

func (warStrategy) Apply(ctx context.Context, req Request) error {
	mgr := req.Spec.EffectiveServiceManager()
	src := filepath.Join(req.ArtifactDir, req.Spec.SourceFile)

	req.Logger.Info("stopping service", "unit", req.Spec.ServiceName, "manager", mgr)
	if err := stopService(ctx, mgr, req.Spec.ServiceName); err != nil {
		return err
	}

	clearParent := true // default per spec §3.2
	if req.Spec.TargetClearParent != nil {
		clearParent = *req.Spec.TargetClearParent
	}
	if clearParent {
		parent := filepath.Dir(req.Spec.TargetPath)
		req.Logger.Info("clearing webapps dir", "dir", parent)
		if err := clearDirContents(parent); err != nil {
			return err
		}
	}

	req.Logger.Info("copying war", "src", src, "dst", req.Spec.TargetPath)
	if err := copyFile(src, req.Spec.TargetPath, req.Spec.TargetMode, req.Spec.TargetOwner); err != nil {
		return err
	}

	req.Logger.Info("starting service", "unit", req.Spec.ServiceName)
	return startService(ctx, mgr, req.Spec.ServiceName)
}
