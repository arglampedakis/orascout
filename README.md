# orascout

**Registry-driven auto-deploy for plain ORAS artifacts.**

`orascout` is a small Linux daemon that watches an OCI registry (Docker Hub, GHCR,
Harbor, etc.) for new versions of [ORAS](https://oras.land/) artifacts and deploys
them to the host. It targets the artifacts you push with `oras push` — JAR files,
WAR files, static dist folders, migration scripts, whatever you can put in an OCI
manifest — and replaces hand-rolled cron-and-scp deploy scripts with a single
generic daemon.

The watcher itself is generic. **Deployment behavior is driven by OCI annotations
attached to the manifest at push time** — so the puller never needs to know about
your services. Push-side tooling (your CI) decides what to deploy and how; pull-side
tooling just executes the contract.

## Status

Early MVP. The annotation schema (see [SPEC.md](SPEC.md)) is the part you'll want to
review first — once that's stable, the daemon implementing it is the easy part.

## Start here

* **[TUTORIAL.md](TUTORIAL.md)** — step-by-step guide that gets three real
  services (JAR, WAR, static `dist/`) auto-deploying from an OCI registry.
  Has two paths: a pure-Docker demo (no Linux server needed, runs on
  Windows / macOS / Linux with Docker Desktop) and a real Linux + systemd
  setup for production.
* [SPEC.md](SPEC.md) — full reference of every annotation key.
* [examples/](examples/) — annotated config + push scripts you can adapt.

## How it works

```
                  ┌────────────────────────┐
   docker push ──►│   OCI registry         │
   oras push      │   (Docker Hub / GHCR)  │
                  └─────────┬──────────────┘
                            │ poll manifest digest
                            ▼
                  ┌────────────────────────┐
                  │   orascout daemon      │   ◄── systemd timer
                  │   on each target host  │
                  └─────────┬──────────────┘
                            │ if digest changed
                            │   1. pull artifact
                            │   2. read annotations
                            │   3. run pre-hook
                            │   4. dispatch strategy
                            │      (jar | war | static | run-once | hook-only)
                            │   5. healthcheck
                            │   6. run post-hook
                            │   7. update state
                            ▼
                  ┌────────────────────────┐
                  │   your service         │
                  └────────────────────────┘
```

## Install

Cross-compile a static binary for the target host:

```bash
cd orascout
go mod tidy
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o orascout ./cmd/orascout
scp orascout target-host:/usr/local/bin/
scp packaging/systemd/orascout.* target-host:/etc/systemd/system/
ssh target-host 'sudo systemctl daemon-reload && sudo systemctl enable --now orascout.timer'
```

Drop a config at `/etc/orascout/config.yaml` (see [examples/config.yaml](examples/config.yaml)).

## Quick example

Push a JAR with the annotations that tell `orascout` how to deploy it:

```bash
oras push docker.io/myorg/foo:latest \
  --artifact-type application/vnd.dev.orascout.artifact.v1+json \
  --annotation "dev.orascout.v1.type=jar" \
  --annotation "dev.orascout.v1.source.file=foo.jar" \
  --annotation "dev.orascout.v1.target.path=/home/tomcat/jar-services/foo.jar" \
  --annotation "dev.orascout.v1.service.name=foo.service" \
  --annotation "dev.orascout.v1.service.manager=systemd-user" \
  --annotation "org.opencontainers.image.title=Foo" \
  --annotation "org.opencontainers.image.version=1.4.2" \
  foo.jar:application/java-archive
```

A target host running `orascout` with `docker.io/myorg/foo:latest` in its
config will, within one poll interval:

1. Notice the manifest digest changed.
2. `oras pull` the artifact to a working directory.
3. `systemctl --user stop foo.service`.
4. Copy the JAR to `/home/tomcat/jar-services/foo.jar`.
5. `systemctl --user start foo.service`.
6. Persist the new digest so the next cycle is a no-op.

See [SPEC.md](SPEC.md) for the full annotation schema and built-in strategies.

## Commands

```bash
orascout run     -c /etc/orascout/config.yaml   # daemon mode (loops forever)
orascout run     -c config.yaml --once          # single cycle, then exit
orascout check   -c config.yaml                 # dry-run: show what would deploy
orascout version
```

## Why this exists

Plenty of teams ship plain build artifacts — JARs, WARs, frontend `dist/` trees,
DB migration jars — and don't want to wrap them in container images just to get
auto-deploy on every push. ORAS makes any OCI registry a generic artifact store;
`orascout` is the daemon that turns that store into a real deployment target on
a Linux host.

## License

Apache-2.0. See [LICENSE](LICENSE).

## Acknowledgements

The polling-and-deploy model is inspired by
[containrrr/watchtower](https://github.com/containrrr/watchtower), which does
the same job for container images. orascout brings that idea to the world of
plain ORAS artifacts. Thanks to that project and its contributors for
demonstrating how clean this pattern can be.
