# CLAUDE.md

Project-level context for Claude Code (and other AI coding assistants — Cursor,
Codex, Aider, Windsurf, Copilot Workspace all read this kind of file).

Aimed at someone (human or AI) opening this repo for the first time. SPEC.md
is the protocol reference; TUTORIAL.md is the end-user guide; this file is the
**lay of the land for contributors**.

---

## What this is

orascout is a registry-driven auto-deploy daemon for plain
[ORAS](https://oras.land/) artifacts. It runs as a systemd timer on a Linux
host, polls an OCI registry, diffs the manifest digest against an on-disk
state file, and on a change pulls the artifact and deploys it.

The architectural decision that makes the whole thing work: **the puller is
generic; per-artifact deploy intent lives on the manifest as OCI annotations**.
A push from CI carries everything the host needs to know (target path, service
unit, strategy). The host never needs per-service config — it just runs orascout
with a list of repos to watch.

Compare:
- **plain `oras pull` in cron**: same registry, same artifacts, but you write
  per-service deploy scripts on every host. orascout removes that work.
- **Per-host config-driven deploy agents**: same end state, but require
  maintaining server-side config files that drift from reality. Annotations
  on the manifest stay in sync because they ship with the artifact.

---

## Directory map

```
cmd/orascout/                CLI entrypoint (run, check, version, help)
pkg/annotations/             ★ ONLY exported package — importable by CI tooling
                             that wants to construct annotation sets without
                             duplicating string constants.
internal/config/             YAML config loader, defaults, env-var expansion
internal/registry/           oras-go/v2 wrapper: resolve digest, fetch manifest
                             annotations, pull artifact, push log file
internal/state/              Atomic-write JSON cache of "last deployed digest"
                             per repo:tag
internal/lockfile/           Best-effort PID lock with stale-reclaim
internal/deploy/             Strategy interface + 5 strategies + hooks +
                             healthcheck. One file per strategy:
                               strategy_jar.go
                               strategy_war.go
                               strategy_static.go
                               strategy_runonce.go
                               strategy_hookonly.go
                             Plus deploy.go (dispatcher), systemd.go (helpers),
                             fsutil.go (copy/chown/chmod plumbing), and
                             safety.go (path validation — see below).
internal/watcher/            Main poll loop. Ties config + registry + state +
                             deploy together. Run() loops, RunOnce() does one cycle.

test/                        Docker-only E2E harness — Windows-friendly. Builds
                             orascout inside an alpine container, pushes a
                             fixture artifact to a local registry, runs the
                             daemon --once, asserts file deployed.
examples/demo/               TUTORIAL.md Part A — quick visual demo (no systemd)
examples/linux-server-demo/  TUTORIAL.md Part B — systemd-in-Docker demo, real
                             OpenJDK 8 + Tomcat 8.5 + nginx
examples/sandbox-server/     Personal rehearsal rig with production-shaped paths
                             (/home/tomcat/jar-services, /opt/tomcat/instances/...).
                             User installs orascout by hand from /staging/ to
                             practice the real-host workflow.
packaging/                   Production systemd unit + timer + install.sh
                             one-liner script.

SPEC.md                      The manifest annotation schema. Load-bearing.
TUTORIAL.md                  Three-part user guide (Part A/B/C).
README.md                    Project pitch + quick start.
```

---

## Load-bearing design decisions

Things that look like arbitrary choices but aren't — change them and something
will break or get worse.

### Annotations on the manifest, not config on the host
The whole reason the watcher stays generic. If you find yourself wanting to add
a "magic file in the artifact root that orascout reads" or a per-service section
in `/etc/orascout/config.yaml`, push back hard — that's the model collapsing
back into the bash-script-with-case-statement we replaced.

### `pkg/annotations` is the ONLY exported package
Everything else is `internal/`. Go's compiler enforces this: no external repo
can `import "github.com/arglampedakis/orascout/internal/..."`. The watcher
internals are deliberately not a stable API. CI tools that want to push valid
annotations import `pkg/annotations` for the string constants and `Parse()`.

### Annotation namespace has a `v1` segment
`dev.orascout.v1.type=jar`, not `dev.orascout.type=jar`. The `v1` is the
forward-compat hatch: when SPEC.md needs a breaking change (e.g. some
annotation key gets renamed), a new `dev.orascout.v2.*` namespace can run
alongside v1 on the same manifest during migration. Pullers prefer the
highest version they understand.

### oras-go library, not shell-out to the `oras` CLI
The bash prototype used the `oras` CLI binary. Reimplementing in Go meant we
could pull in [oras.land/oras-go/v2](https://pkg.go.dev/oras.land/oras-go/v2)
and produce a single static binary with no runtime deps beyond `systemctl`.
Don't reintroduce a shell-out to `oras` — it adds an install dependency and
makes the deploy hosts heavier.

### `service.manager=none` exists
Lets the `jar`/`war` strategies skip `systemctl` calls entirely (just copy the
file). This is the path the E2E test and the static deploys in
`examples/demo/` use. Without it, you couldn't test the strategies anywhere
that doesn't have systemd, which would mean tests only run on Linux VMs.

### Manifest paths are untrusted input — safety.go is the gate
Anyone with push access to a watched repo controls the annotations, so
`target.path=/` with `clear=true` would otherwise wipe the host.
`internal/deploy/safety.go` enforces, BEFORE any side effect (service stop,
hook, copy, clear): a built-in denylist of system paths (never
configurable away), a minimum depth of 2 for clear operations, symlink
resolution before clearing, containment of source/hook paths inside the
artifact dir, and regular-files-only copying. The operator allowlist
(`allowed_target_roots` in config) is the real security boundary — the
denylist only stops catastrophic/obvious cases. Never add a write/clear
primitive that bypasses these checks; `clearDirContents` calls
`guardClearDir` itself as defense in depth. SPEC.md §10 is the normative
description — keep the two in sync.

### State updated ONLY on full success
`Run()` -> `cycleOne()` -> `deploy.Run()` -> `state.Set()`. If any step in
between fails, `state.Set()` doesn't run, so the next cycle will retry. There
is intentionally no auto-rollback (see "Out of scope" below).

### Lockfile is "best-effort" — reclaims stale same-PID locks
We don't use `flock`/`fcntl`; we write the PID and check liveness with
`syscall.Signal(0)`. This is enough for systemd-timer overlap, which is the
realistic adversary. It is NOT enough for malicious double-fires. If you ever
need real exclusion, swap to `golang.org/x/sys/unix.Flock`.

---

## Conventions

### Errors
Always wrap with context: `fmt.Errorf("describe step: %w", err)`. The watcher
logs at INFO level on success and ERROR level on cycle failure; the wrapping
chain is what makes the error line useful.

### Logging
Single `*slog.Logger` instance constructed in `main.go`, passed down via the
`Logger` field on `watcher.Watcher` and `deploy.Request`. We use `slog`'s
key/value form (`logger.Info("msg", "key", value)`), not `Printf`-style.

### Tests
- Pure-Go unit tests: `go test ./...`. Fast, no network, no Docker.
- E2E: `test/e2e.sh` against a local `registry:2` (see `test/README.md`).
- One test file per package. Use `t.TempDir()` — never global state.
- `go vet` is stricter than `go build` about unused vars. CI runs both;
  local edits should `go vet ./...` before commit.

### Strategy files
One file per strategy in `internal/deploy/strategy_*.go`. Each is small (one
type, one method). Adding a sixth strategy means: new file + entry in
`Dispatch()` in `deploy.go` + new SPEC.md section + parser support in
`pkg/annotations/annotations.go`. Don't add strategies as case branches in a
single file.

---

## Build / test / release

```bash
# unit tests
go test ./...

# end-to-end (host needs Go + oras + jq + docker)
./test/e2e.sh

# end-to-end (Docker only — Windows-friendly)
docker compose -f test/docker-compose.yml up --build \
  --abort-on-container-exit --exit-code-from e2e

# manual cross-compile (single binary)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o orascout ./cmd/orascout

# release
git tag v0.X.Y && git push origin v0.X.Y
# Goreleaser runs via .github/workflows/release.yml and publishes
# linux/amd64 + linux/arm64 binaries to GitHub Releases.
# Alternative trigger: Actions UI -> release.yml -> "Run workflow"
# (workflow_dispatch fallback for when tag-trigger misbehaves).
```

---

## Gotchas we've already hit

Things that cost time once. Worth knowing before they bite again.

- **Git Bash MSYS path translation on Windows.** Paths starting with `/` in
  `docker run` / `docker exec` arguments get rewritten to
  `C:/Program Files/Git/...`. Prefix the command with `MSYS_NO_PATHCONV=1` or
  use PowerShell. Affects `docker exec linux-server cat /srv/jars/hello.jar`,
  `docker run -v "$PWD:/src" -w /src ...`, anything similar.

- **goreleaser v2 syntax.** `archives.format: binary` (singular) was renamed
  to `archives.formats: [binary]` (plural list). Either fails or warns in v2.

- **GitHub Actions resolver flake.** Major-version tags like
  `actions/setup-go@v5` sometimes resolve to a SHA that's been deleted
  upstream, returning a 500 from `codeload.github.com`. Fix: pin to a specific
  minor (`@v5.0.2`). Same for `goreleaser/goreleaser-action@v6` -> `@v6.0.0`.

- **systemd in Docker.** Requires `privileged: true` + tmpfs `/run`+`/run/lock`
  + bind mount `/sys/fs/cgroup`. Only used in `examples/linux-server-demo/`
  and `examples/sandbox-server/` for testing — never appropriate for prod
  containers (and orascout is not designed to run inside a container in prod
  anyway; it's a host daemon).

- **Tomcat WAR redeploy needs `target.clear-parent=true`.** Without it, the
  old exploded `ROOT/` directory sits next to the new `ROOT.war` and Tomcat
  picks the wrong one. Always set `target.clear-parent=true` for `type=war`
  unless you have a specific reason not to.

- **GitHub Releases on a private repo need auth in the curl.** The
  `releases/latest/download/*` URL works without auth only on public repos.
  For private: `curl -H "Authorization: Bearer $PAT" ...`. Once orascout is
  flipped to public, the install one-liner works as-is.

- **`go vet` catches unused vars in test files that `go build` lets slide.**
  Always `go vet ./...` before pushing.

- **Newer Alpine (3.24+) dropped the standalone `wget` package.** `apk add
  wget` fails; the busybox wget applet in the base image still exists. We
  avoid the issue entirely by not downloading tools in-build (next item).

- **Corporate VPNs can break container egress — especially BuildKit's.**
  Symptom: `apk`/`wget`/`curl` inside `docker build` fail with TLS errors
  ("TLS: unspecified error", "TLS handshake timeout") while the host and
  sometimes `docker run` containers work. Mitigations used here: get CLI
  tools via `COPY --from=<official image>` (pulls go through the daemon,
  e.g. `ghcr.io/oras-project/oras:v1.2.0` for the oras CLI) instead of
  in-build downloads; `DOCKER_BUILDKIT=0` falls back to the legacy builder
  with normal bridge networking; `docker pull` with retries pre-warms the
  local image cache. If everything fails, the network is flapping — wait.

- **oras-go >= 2.6 requires Go >= 1.25.** The `go` directive in go.mod,
  `go-version` in both workflows, and every `golang:X-alpine` builder image
  must move together when bumping it.

---

## Out of scope (intentional, not gaps)

- **No auto-rollback on failed deploy.** If a deploy fails, state is not
  updated, so the next cycle retries the same push. To roll back, push the
  previous artifact again — the digest changes, orascout treats it as new.
  Building rollback in would mean keeping previous versions on the host,
  which we don't.

- **No multi-arch in a single artifact.** Each manifest = one host arch.
  If you need amd64 + arm64, push two repos (or two tags). OCI image-index
  manifests would change this; the parser doesn't handle them yet.

- **Polling only — no webhooks.** Webhook-driven deploys are faster but
  require open inbound ports on every host. Polling is uglier but works
  through every firewall. Webhooks could be added as an optional extra
  trigger, not as a replacement.

- **No digest-pinned tags.** `repo:latest` is what you watch. If you want to
  pin to an immutable digest, that's an extension (a new annotation or a
  config field).

- **No log shipping beyond the optional `logs_push` feature.** orascout
  writes a local log file and can push it back to the registry as an
  artifact. It does not integrate with journald structured logging, nor
  with external log aggregators.

---

## When something feels wrong

If a code change starts feeling like it's fighting the design — most often
shape: "I need to put per-service config in `config.yaml` so orascout knows
how to deploy X" — that's a smell. The right shape is almost always: **add an
annotation** (and update SPEC.md), not add a config knob.

When the annotation approach genuinely doesn't fit, the escape hatch is
`type=hook-only` with a script bundled in the artifact. That's preferable to
adding logic to the watcher.
