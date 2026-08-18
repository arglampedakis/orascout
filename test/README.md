# orascout — local testing

Three tiers, by feedback speed.

## 1. Go unit tests (fastest — milliseconds)

```bash
cd orascout
go test ./...
```

Requires Go on the host. Covers the annotation parser, state store, lockfile,
config loader, and small helpers. No network, no containers.

## 2. End-to-end against a local registry (recommended)

This spins up a real OCI registry, pushes a fixture artifact with deploy
annotations, runs `orascout run --once`, and asserts the artifact landed at
the configured target path.

You have two ways to run it depending on what's installed on your host.

### 2a. Full Docker (Windows-friendly — only Docker required)

Everything happens inside containers. No Go, `oras`, `jq`, or bash on the host
is needed.

```powershell
# from the orascout/ directory
docker compose -f test/docker-compose.yml up --build `
  --abort-on-container-exit --exit-code-from e2e
docker compose -f test/docker-compose.yml down -v
```

Same thing on Linux/macOS/WSL (using bash line-continuations):

```bash
cd orascout
docker compose -f test/docker-compose.yml up --build \
  --abort-on-container-exit --exit-code-from e2e
docker compose -f test/docker-compose.yml down -v
```

What this does:

1. Builds the `e2e` image (Go + `oras` CLI + `jq` + bash, pinned via
   [test/Dockerfile.test](Dockerfile.test)).
2. Brings up `registry:2` and waits for its healthcheck.
3. Runs `test/e2e.sh` inside the `e2e` container against
   `registry:5000` over the compose network.
4. Exits 0 if all assertions pass, nonzero otherwise; `--exit-code-from e2e`
   forwards that status to the `docker compose` invocation so CI can detect
   failures.

### 2b. Hybrid (Linux/macOS/WSL with Go + `oras` + `jq` locally)

Faster iteration on Go code since you skip the e2e image build.

```bash
cd orascout
docker compose -f test/docker-compose.yml up -d registry
./test/e2e.sh                        # talks to localhost:5000
docker compose -f test/docker-compose.yml down -v
```

### What the e2e script asserts

1. Push fixture v1 → `orascout --once` → target file exists with v1 content.
2. Push fixture v2 → `orascout --once` → target file updates to v2 content.
3. `orascout --once` again → target file's **mtime does not change**
   (proves the digest-diff / no-op path works).

The fixture uses `dev.orascout.v1.service.manager=none`, so the `jar`
strategy skips `systemctl` and just copies bytes — giving full coverage of
the registry + annotations + pull + deploy path without needing real
systemd services.

If a step fails, the tmp directory is preserved and printed so you can
inspect the orascout log, state file, and pulled artifact.

## 3. Real systemd target host (final smoke test)

For the most realistic test — orascout under a systemd timer, deploying to
real services — spin up a fresh Linux VM (or LXC container) and:

```bash
curl -fsSL https://raw.githubusercontent.com/arglampedakis/orascout/main/packaging/install.sh | sudo bash
# edit /etc/orascout/config.yaml
sudo systemctl enable --now orascout.timer
journalctl -u orascout.service -f
```

Push a real artifact for one of your services with the appropriate
annotations and watch it deploy.
