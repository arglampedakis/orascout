package deploy

import "context"

type hookOnlyStrategy struct{}

// hookOnly is a no-op strategy; pre/post hooks (run by deploy.Run around the
// strategy) are the entirety of the deploy.
func (hookOnlyStrategy) Apply(ctx context.Context, req Request) error {
	req.Logger.Info("hook-only strategy: nothing to do beyond pre/post hooks")
	return nil
}
