# Contributing to orascout

Thanks for your interest! orascout is a small, focused tool — a generic
registry watcher whose deploy behaviour is driven entirely by manifest
annotations. Contributions that keep it small and focused are the most
likely to land.

## Before you start

* Read [SPEC.md](SPEC.md) — the annotation schema is the project's public
  contract. Most changes touch it.
* Read [CLAUDE.md](CLAUDE.md) — the lay of the land: directory map, the
  load-bearing design decisions, and the conventions. It's written for AI
  coding assistants but it's equally the contributor onboarding doc.
* For anything non-trivial, open an issue first to discuss the approach —
  it avoids wasted work on PRs that fight the design.

## Development setup

You need either **Go 1.22+** or just **Docker** (every check runs in a
container too).

```bash
# unit tests (fast, no network)
go test ./...

# format + vet — CI enforces both
gofmt -l .
go vet ./...

# end-to-end test against a real local registry (needs Docker; also needs
# the oras CLI + jq on the host)
./test/e2e.sh

# the same E2E fully inside Docker (nothing on the host but Docker):
docker compose -f test/docker-compose.yml up --build \
  --abort-on-container-exit --exit-code-from e2e
```

See [test/README.md](test/README.md) for details, and
[examples/](examples/) for three docker-compose stacks that exercise the
daemon end-to-end (including a systemd-in-Docker "fake Linux server").

## Rules of the road

These mirror the design decisions in [CLAUDE.md](CLAUDE.md) — PRs that
break them will get pushback:

* **Deploy intent lives on the manifest, not in host config.** If your
  change adds per-service knowledge to `config.yaml` or the watcher, it's
  probably the wrong shape — add an annotation instead (and update SPEC.md).
* **Annotation changes are three-part changes**: SPEC.md + the parser in
  `pkg/annotations` + tests. All in the same PR.
* **New deploy strategy = new file.** `internal/deploy/strategy_<name>.go`,
  an entry in `Dispatch()`, a SPEC.md section, parser support, tests. Don't
  grow existing strategies with special cases.
* **Never bypass the path-safety layer.** All writes/clears go through the
  checks in `internal/deploy/safety.go` (see SPEC.md §10). Loosening the
  denylist needs a strong, documented justification.
* **Only `pkg/annotations` is public API.** Everything under `internal/`
  can change freely between releases.

## Pull requests

1. Fork, create a topic branch from `main`.
2. Keep PRs small and single-purpose.
3. Make sure `gofmt -l .` is clean and `go vet ./...` + `go test ./...`
   pass — CI runs exactly these.
4. Say in the PR description what you tested and how (unit tests? e2e? one
   of the example stacks?).
5. Commit messages: short imperative subject, `feat:`/`fix:`/`docs:`/`ci:`
   prefixes appreciated but not enforced.

## Releases

Maintainers cut releases by pushing a `vX.Y.Z` tag; goreleaser builds and
publishes linux/amd64 + linux/arm64 binaries automatically. Contributors
don't need to do anything release-related in PRs.

## Security issues

Please do **not** open public issues for vulnerabilities — see
[SECURITY.md](SECURITY.md).
