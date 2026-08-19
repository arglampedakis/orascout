## What this PR does

<!-- One or two sentences. Link the issue if there is one. -->

## How it was tested

<!-- go test? ./test/e2e.sh? one of the examples/ stacks? Describe briefly. -->

## Checklist

- [ ] `gofmt -l .` is clean, `go vet ./...` and `go test ./...` pass
- [ ] If annotations changed: SPEC.md, `pkg/annotations`, and tests updated together
- [ ] If deploy behaviour changed: path-safety rules (SPEC.md §10 / `internal/deploy/safety.go`) still hold
- [ ] Docs updated where behaviour changed (README / TUTORIAL / examples)
