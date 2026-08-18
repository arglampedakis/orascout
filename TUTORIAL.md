# orascout — step-by-step tutorial

End-to-end walkthrough that takes you from zero to **three live services
auto-deploying from an OCI registry**: one JAR, one WAR, and one static
frontend `dist/`.

There are three paths, in increasing fidelity to production:

- **[Part A](#part-a--all-docker-demo-no-linux-server-needed)** — pure
  docker-compose: registry + orascout + nginx + three "deploy target"
  volumes. No systemd, no real JVM, no Tomcat. Fastest way to *see* the
  push → poll → deploy → observe loop. ~5 minutes start to finish.

- **[Part B](#part-b--fake-linux-server-in-docker)** — a single privileged
  container that boots **real systemd** as PID 1, with **real OpenJDK 8 +
  Apache Tomcat 8.5 + nginx + a real Java HTTP service**. orascout runs
  under a systemd timer and drives real `systemctl stop`/`start` around
  each deploy. Same systemd units and config files you'd use on a real
  host — just no host needed. ~5 minutes after the first image build.

- **[Part C](#part-c--real-linux-server)** — installs orascout on an actual
  Linux server running systemd, with your own real JAR / WAR / `dist/`
  artifacts. This is how you'd actually run it in production.

Read Part A first even if you're going straight to a real server. The
push/poll/deploy loop is the same in all three parts; Part A just makes
it the most visible. Part B then verifies the real-systemd path works on
your machine before you commit to a host setup in Part C.

---

## Part A — all-Docker demo (no Linux server needed)

### What you'll see

A docker-compose stack with four containers:

| Container | Role                                                            |
|-----------|-----------------------------------------------------------------|
| `registry`| Local OCI registry on `localhost:5000`.                         |
| `pusher`  | One-shot helper that pushes the v1 artifacts and exits.         |
| `orascout`| The orascout daemon. Polls the registry every 10s and deploys. |
| `nginx`   | Serves the static `dist/` volume at `http://localhost:8080`.    |

The three "deploy targets" are Docker volumes shared with the orascout
container:

* `/srv/jars/hello.jar`   ← orascout writes the JAR here.
* `/srv/webapps/ROOT.war` ← orascout writes the WAR here.
* `/srv/dist/`            ← orascout syncs the static files here; nginx reads
                            from the same volume.

When you push a **v2** of any artifact, orascout notices on its next 10-second
cycle and rewrites the target. For the static deploy, your browser refresh
shows the change immediately. For the jar/war, you'll see the file replaced
via `docker exec ls`.

The fixtures use `service.manager=none` so orascout doesn't try to run
`systemctl` inside the container — it just performs the file deploy. On a
real host you'd use `systemd-user` or `systemd` and orascout would also
restart the service unit.

### Prereqs

- Docker Desktop (Windows / macOS / Linux) **— that's the only requirement.**

You do **not** need Go, the `oras` CLI, `jq`, bash, or any registry login.
Everything that needs those tools runs inside containers.

### Step 1 — bring up the stack

From the orascout repo root:

```powershell
docker compose -f examples/demo/docker-compose.yml up --build -d
```

First run takes ~2 minutes (downloads `golang:1.22-alpine`, builds orascout,
builds the test image, pulls `registry:2` and `nginx:alpine`). Subsequent
brings-up are seconds.

When `up --build -d` returns, `docker compose ps` should show:

```
NAME                       STATUS
orascout-demo-registry     Up (healthy)
orascout-demo-pusher       Exited (0)              ← one-shot, has done its job
orascout-demo-daemon       Up
orascout-demo-nginx        Up
```

If `pusher` is still `Up`, it's still pushing — wait a few seconds. If
`orascout-demo-daemon` is missing because the build failed, see the
[troubleshooting section](#troubleshooting) below.

### Step 2 — watch the first deploy

Stream orascout's log:

```bash
docker compose -f examples/demo/docker-compose.yml logs -f orascout
```

You'll see something like:

```
orascout-demo-daemon  | level=INFO msg="orascout starting" poll_interval=10s repos=3
orascout-demo-daemon  | level=INFO msg="new artifact (first deploy)" ref=registry:5000/orascout-demo/hello-jar:latest digest=sha256:...
orascout-demo-daemon  | level=INFO msg="copying jar" src=/var/lib/orascout/artifacts/hello-jar/hello.jar dst=/srv/jars/hello.jar
orascout-demo-daemon  | level=INFO msg=deployed type=jar
orascout-demo-daemon  | level=INFO msg="new artifact (first deploy)" ref=registry:5000/orascout-demo/hello-war:latest ...
orascout-demo-daemon  | level=INFO msg="clearing webapps dir" dir=/srv/webapps
orascout-demo-daemon  | level=INFO msg="copying war" ...
orascout-demo-daemon  | level=INFO msg=deployed type=war
orascout-demo-daemon  | level=INFO msg="new artifact (first deploy)" ref=registry:5000/orascout-demo/hello-dist:latest ...
orascout-demo-daemon  | level=INFO msg="syncing static directory" src=...  dst=/srv/dist  clear=true
orascout-demo-daemon  | level=INFO msg=deployed type=static
```

Press `Ctrl+C` to stop tailing logs (the daemon keeps running).

### Step 3 — verify each deploy

> **Git Bash on Windows users:** MSYS translates leading `/` in command
> arguments to `C:/Program Files/Git/...` before passing them to Docker.
> The `cat /srv/...` commands below will fail with "No such file or
> directory" pointing at a `C:/Program Files/Git/srv/...` path. Either
> prefix the command with `MSYS_NO_PATHCONV=1`, or use a double slash
> (`cat //srv/jars/hello.jar`), or run these commands from PowerShell / cmd
> where this translation doesn't happen.

**JAR** — should be the v1 fixture content:

```bash
docker compose -f examples/demo/docker-compose.yml exec orascout cat /srv/jars/hello.jar
```

Expected: `orascout demo — JAR fixture v1. ...`

**WAR** — same idea, deployed under the webapps dir:

```bash
docker compose -f examples/demo/docker-compose.yml exec orascout cat /srv/webapps/ROOT.war
```

Expected: `orascout demo — WAR fixture v1. ...`

**Static** — open [http://localhost:8080](http://localhost:8080) in a
browser. You should see a blue-tinted "Hello from orascout 👋 v1" page.

If all three look right, the deploy loop works.

### Step 4 — push v2 and watch live redeploy

Re-run the pusher with the `v2` argument:

```bash
docker compose -f examples/demo/docker-compose.yml run --rm pusher v2
```

This pushes three new manifests with new digests. Within ~10 seconds (the
poll interval), orascout's log will show:

```
level=INFO msg="artifact changed" ref=registry:5000/orascout-demo/hello-dist:latest old=sha256:abc... new=sha256:def...
level=INFO msg="syncing static directory" ...
level=INFO msg=deployed type=static
```

Verify:

```bash
docker compose -f examples/demo/docker-compose.yml exec orascout cat /srv/jars/hello.jar     # now says "v2"
```

Refresh [http://localhost:8080](http://localhost:8080). The page background
should be **amber** now, with the "v2" callout.

That's the full loop:

```
your CI / a developer       orascout daemon         your "service"
        │                          │                       │
        │ oras push ──► registry ──┤                       │
        │                          │ poll every 10s        │
        │                          │ sees new digest       │
        │                          │ pulls artifact        │
        │                          │ reads annotations     │
        │                          │ runs strategy ────────┤
        │                          │                       │ file replaced
        │                          │                       │ (no restart needed for static;
        │                          │                       │  systemctl restart for jar/war
        │                          │                       │  on a real host)
```

### Step 5 — clean up

```bash
docker compose -f examples/demo/docker-compose.yml down -v
```

The `-v` removes the volumes, so the next `up` starts from a clean slate.

### Troubleshooting

* **Stack starts but orascout never deploys:** check the orascout container
  is healthy and the registry healthcheck passed.
  `docker compose -f examples/demo/docker-compose.yml ps`.
* **Pusher fails:** look at `docker compose ... logs pusher`. Most likely
  the registry wasn't healthy yet — `pusher` has `depends_on: registry`
  with `condition: service_healthy`, but firewalls or slow IO can defeat
  this. Retry with a manual `pusher` run after a few seconds.
* **`localhost:8080` shows the nginx default page**, not the orascout HTML:
  the static deploy hasn't happened yet (or failed). Check the orascout
  log for an error on the `hello-dist` repo.
* **Port 5000 / 8080 conflict on Windows:** Docker Desktop reserves these
  on some hosts. Edit `examples/demo/docker-compose.yml` to use a free
  port (e.g. `15000:5000`, `18080:80`).

---

## Part B — fake Linux server in Docker

A faithful simulation of Part C without needing a Linux host: a single
privileged container that runs **systemd as PID 1**, with **OpenJDK 8,
Apache Tomcat 8.5.100, nginx, and a real Java HTTP service**. orascout is
installed inside it as a real systemd `.service` + `.timer`, and it drives
real `systemctl stop`/`start` around each deploy. The systemd units, the
nginx config, and `/etc/orascout/config.yaml` are byte-for-byte the same
files you'd put on a real Ubuntu server in Part C.

This is the place to validate the real-systemd path on your laptop before
you commit to a real host. Everything in this section runs from `docker
compose`.

### What's in the box

```
┌─────────────────────────────────────────────────┐
│ linux-server container  (PID 1 = systemd)       │
│                                                 │
│  ┌──────────────────┐  ┌──────────────────┐    │
│  │ orascout.timer   │─▶│ orascout.service │    │ every 30s
│  │ (every 30s)      │  │ (oneshot)        │    │
│  └──────────────────┘  └────────┬─────────┘    │
│                                 │ deploys      │
│                                 ▼              │
│  /opt/orascout/jars/hello.jar  → hello-jar.svc │
│  /opt/tomcat/webapps/ROOT.war  → tomcat.svc    │
│  /var/www/html/index.html      → nginx (no rs) │
│                                                 │
│  Exposed ports:                                 │
│    :80   nginx       (static demo)              │
│    :8080 Tomcat 8.5  (WAR demo)                 │
│    :8082 hello-jar   (Java HTTP server)         │
└─────────────────────────────────────────────────┘
       ▲                              ▲
       │ pulls artifacts              │
       │                              │
┌──────┴─────────┐          ┌─────────┴────────┐
│ registry:2     │◀─────────│ pusher           │
│                │  pushes  │ (one-shot, JDK 8 │
│                │          │  builds real JAR │
│                │          │  + WAR fixtures) │
└────────────────┘          └──────────────────┘
```

The `pusher` container compiles real JAR and WAR fixtures with JDK 8 at
image-build time, so the services in the linux-server actually start and
serve HTTP — you can `curl` them and see the version change after each
push.

### Prereqs

- Docker Desktop on Windows / macOS, or Docker Engine on Linux. **That's it.**
  No Go, no `oras` CLI, no JDK on your host.

> **Warning:** the linux-server container uses `privileged: true` because
> systemd inside an unprivileged container can't manage cgroups. That's
> fine for a local test rig but never appropriate for production. The
> real-host setup in Part C does NOT need privileged mode.

### Step 1 — bring up the stack

```powershell
docker compose -f examples/linux-server-demo/docker-compose.yml up --build -d
```

First-time build takes ~3-5 minutes (downloads `golang`, `eclipse-temurin:8-jdk`,
`registry:2`, `jrei/systemd-ubuntu:22.04`, Tomcat 8.5.100, alpine + oras CLI;
compiles JAR/WAR fixtures; compiles orascout). Subsequent runs reuse
cached layers and start in seconds.

When `up` returns, `docker compose ps` should show:

```
NAME                           STATUS
orascout-lsd-registry          Up (healthy)
orascout-lsd-pusher            Exited (0)        ← one-shot, has finished
orascout-lsd-linux-server      Up (healthy)
```

### Step 2 — watch the first deploy via systemd journal

orascout's first cycle fires 15s after the linux-server container boots,
then every 30s. Stream the orascout journal:

```bash
docker compose -f examples/linux-server-demo/docker-compose.yml \
  exec linux-server journalctl -u orascout.service -f
```

You'll see real systemd-driven log lines:

```
orascout[42]: msg="new artifact (first deploy)" ref=registry:5000/orascout-demo/hello-jar:latest
orascout[42]: msg="stopping service" unit=hello-jar.service manager=systemd
orascout[42]: msg="copying jar" src=...  dst=/opt/orascout/jars/hello.jar
orascout[42]: msg="starting service" unit=hello-jar.service
orascout[42]: msg=deployed type=jar

orascout[42]: msg="stopping service" unit=tomcat.service manager=systemd
orascout[42]: msg="clearing webapps dir" dir=/opt/tomcat/webapps
orascout[42]: msg="copying war" ...
orascout[42]: msg="starting service" unit=tomcat.service
orascout[42]: msg=deployed type=war

orascout[42]: msg="syncing static directory" dst=/var/www/html clear=true
orascout[42]: msg=deployed type=static
```

`Ctrl+C` to stop tailing.

### Step 3 — verify each service is actually serving

All three should now serve their v1 content. From your host:

```bash
curl http://localhost:8082          # JAR HTTP server
# -> Hello from JAR v1

curl http://localhost:8081          # Tomcat-served WAR
# -> <h1>Hello from v1 (WAR)</h1>   (with HTML chrome)

curl http://localhost:8080          # nginx-served static
# -> <h1>Hello from v1 (static)</h1>
```

Open the same URLs in a browser for the colored versions. You can also
poke at systemd directly:

```bash
docker compose -f examples/linux-server-demo/docker-compose.yml exec linux-server \
  systemctl status tomcat.service hello-jar.service orascout.timer
```

### Step 4 — push v2 and watch the live redeploy

```bash
docker compose -f examples/linux-server-demo/docker-compose.yml run --rm pusher v2
```

Within ~30 seconds the timer fires, the journal shows three "artifact
changed" entries followed by three "deployed" lines, and:

```bash
curl http://localhost:8082          # -> Hello from JAR v2
curl http://localhost:8081          # -> Hello from v2 (WAR)
curl http://localhost:8080          # -> Hello from v2 (static)
```

The JAR service's HTTP response changes because orascout actually
`systemctl stop`-ped it, replaced `/opt/orascout/jars/hello.jar`, and
`systemctl start`-ed it. Same story for Tomcat — it reloaded the new
WAR. The static deploy needed no service restart at all; nginx just
serves whatever's in `/var/www/html`.

### Step 5 — tear down

```bash
docker compose -f examples/linux-server-demo/docker-compose.yml down -v
```

### Mapping this back to a real Linux server

Every file under
[`examples/linux-server-demo/linux-server/`](examples/linux-server-demo/linux-server/)
corresponds to a file you'd put on a real Ubuntu host:

| In the demo container                                  | On a real host                                                   |
|--------------------------------------------------------|------------------------------------------------------------------|
| `/etc/orascout/config.yaml` (from `orascout-config.yaml`) | `/etc/orascout/config.yaml`                                       |
| `/etc/systemd/system/orascout.service` + `.timer`      | Same path — copy from [packaging/systemd/](packaging/systemd/)   |
| `/etc/systemd/system/hello-jar.service`                | Same path — yours to write, matching your service                |
| `/etc/systemd/system/tomcat.service`                   | Same path — typically installed by your distro or by hand        |
| `/etc/nginx/sites-available/default`                   | Same path on Debian/Ubuntu; `/etc/nginx/conf.d/*.conf` on RHEL   |
| `/usr/local/bin/orascout`                              | Same path — from `install.sh` or a manual `scp`                  |

Once the Part B demo runs cleanly, the same files drive a real production
host. The only things that change in Part C are `registry_prefix` in
`orascout-config.yaml` (point at Docker Hub / GHCR / your private registry
instead of `registry:5000`) and the registry credentials (via
`/etc/orascout/env` referenced by `EnvironmentFile=`).

For more detail (troubleshooting, ports table, the systemd-in-container
caveats), see
[examples/linux-server-demo/README.md](examples/linux-server-demo/README.md).

---

## Part C — real Linux server

### What you'll do

Stand up the same three services on a real Linux host. orascout runs under
a systemd timer, polls a registry of your choice (Docker Hub, GHCR, Harbor,
private), and on each new push performs the real deploy: stop systemd unit,
copy the artifact, start systemd unit.

### Prereqs

* A Linux host with `systemd` (any modern distro). You'll need shell access
  with `sudo`.
* A writable registry where you can `oras push`. Free options:
  Docker Hub (private repo), GHCR, or self-host with the same
  `registry:2` you used in Part A.
* On your **build / CI side** (could be the same machine, could be your
  laptop, could be a CI runner): the [`oras` CLI](https://oras.land/docs/installation/).
* Your three real artifacts: a `.jar`, a `.war`, and a built frontend `dist/`
  directory. If you don't have these yet, the demo fixtures from Part A will
  still work — the strategies don't validate file contents.

### Step 1 — install orascout on the target host

Once you've published a release (see [.goreleaser.yaml](.goreleaser.yaml) +
[.github/workflows/release.yml](.github/workflows/release.yml)) the install
is a one-liner:

```bash
curl -fsSL https://github.com/arglampedakis/orascout/releases/latest/download/orascout-linux-amd64 \
  -o /usr/local/bin/orascout
sudo chmod +x /usr/local/bin/orascout
orascout version
```

Until that release exists, build locally and `scp`:

```bash
# on any host with Go installed
cd orascout
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o orascout ./cmd/orascout
scp orascout your-server:/tmp/
ssh your-server 'sudo mv /tmp/orascout /usr/local/bin/ && sudo chmod +x /usr/local/bin/orascout'
```

### Step 2 — install systemd units

```bash
ssh your-server
sudo mkdir -p /etc/orascout /var/lib/orascout /var/log/orascout
# from your repo checkout:
scp packaging/systemd/orascout.service your-server:/tmp/
scp packaging/systemd/orascout.timer   your-server:/tmp/
ssh your-server 'sudo mv /tmp/orascout.* /etc/systemd/system/ && sudo systemctl daemon-reload'
```

Edit the unit to run as the right user. The shipped
[orascout.service](packaging/systemd/orascout.service) defaults to
`User=tomcat` because that's a common case for JAR/WAR deploys — if your
services run as a different user, change both `User=` and `Group=`.

### Step 3 — config file

Create `/etc/orascout/config.yaml`:

```yaml
# Where your artifacts live. No scheme — just host/namespace.
registry_prefix: docker.io/youruser     # or ghcr.io/youruser, etc.

poll_interval: 1m                       # adjust to taste; 30s–5m is reasonable

# Three repos to watch. Tag defaults to :latest if omitted.
repos:
  - hello-jar:latest
  - hello-war:latest
  - hello-dist:latest

# Registry credentials (optional for public repos).
# Values starting with $ are expanded from the systemd unit's
# EnvironmentFile so secrets never touch this file.
auth:
  username: $REGISTRY_USERNAME
  password: $REGISTRY_TOKEN       # PAT in the password slot (basic-auth convention)

artifacts_dir: /var/lib/orascout/artifacts
state_file:    /var/lib/orascout/state.json
lock_file:     /var/run/orascout.lock
log_file:      /var/log/orascout/orascout.log
```

If you set `auth.*` to `$REGISTRY_USERNAME` / `$REGISTRY_TOKEN`, also
create `/etc/orascout/env`:

```bash
REGISTRY_USERNAME=youruser
REGISTRY_TOKEN=dckr_pat_xxxxxxxxxxxxxxxxxxxxxxxx
```

> `REGISTRY_TOKEN` is a Personal Access Token, not your account password.
> Generate one in your registry's account-settings UI (Docker Hub:
> *Account Settings → Security*; GHCR: a fine-grained PAT with
> `read:packages`; Harbor: a robot account credential). orascout
> passes the PAT in the basic-auth password slot because that's what
> these registries' HTTP APIs expect — semantically it's still a token.

```bash
sudo chmod 600 /etc/orascout/env
```

The shipped systemd unit already includes `EnvironmentFile=-/etc/orascout/env`,
so this gets picked up automatically.

### Step 4 — prepare host paths for the three services

This is the bit you have to think about for **your** services. Decide:

| Service     | What the artifact is | Where it lives on the host                   | systemd unit name              |
|-------------|----------------------|----------------------------------------------|--------------------------------|
| JAR backend | `hello.jar`          | `/home/tomcat/jar-services/hello.jar`        | `hello-jar.service`            |
| WAR backend | `ROOT.war`           | `/opt/tomcat/instances/hello/webapps/ROOT.war` | `hello-war.service`          |
| Frontend    | `dist/` tree         | `/var/www/html/hello-ui/`                    | *(none — nginx serves files)* |

The systemd units for the **jar** and **war** services are yours to write —
they're the ones orascout will stop and start around the file copy. A
minimal example for the JAR:

```ini
# /etc/systemd/system/hello-jar.service  (or place in ~/.config/systemd/user/ for systemd --user)
[Unit]
Description=Hello JAR service
After=network.target

[Service]
Type=simple
ExecStart=/usr/bin/java -jar /home/tomcat/jar-services/hello.jar
Restart=on-failure
User=tomcat

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable hello-jar.service
```

Repeat for the WAR (running tomcat, kafka, whatever you have). The frontend
doesn't need a systemd unit — nginx (or any HTTP server) just serves the
directory orascout writes into.

### Step 5 — push the three artifacts

On your build machine (with the `oras` CLI), set the registry host:

```bash
export REG=docker.io/youruser     # match config.yaml's registry_prefix
```

**JAR:**

```bash
oras push "${REG}/hello-jar:latest" \
  --artifact-type application/vnd.dev.orascout.artifact.v1+json \
  --annotation "dev.orascout.v1.type=jar" \
  --annotation "dev.orascout.v1.source.file=hello.jar" \
  --annotation "dev.orascout.v1.target.path=/home/tomcat/jar-services/hello.jar" \
  --annotation "dev.orascout.v1.service.name=hello-jar.service" \
  --annotation "dev.orascout.v1.service.manager=systemd" \
  --annotation "org.opencontainers.image.title=Hello JAR" \
  --annotation "org.opencontainers.image.version=1.0.0" \
  /path/to/your/hello.jar:application/java-archive
```

If your service is managed by **rootless** systemd (`systemctl --user`),
swap `service.manager=systemd` for `service.manager=systemd-user`.

**WAR:**

```bash
oras push "${REG}/hello-war:latest" \
  --artifact-type application/vnd.dev.orascout.artifact.v1+json \
  --annotation "dev.orascout.v1.type=war" \
  --annotation "dev.orascout.v1.source.file=ROOT.war" \
  --annotation "dev.orascout.v1.target.path=/opt/tomcat/instances/hello/webapps/ROOT.war" \
  --annotation "dev.orascout.v1.target.clear-parent=true" \
  --annotation "dev.orascout.v1.service.name=hello-war.service" \
  --annotation "dev.orascout.v1.service.manager=systemd" \
  --annotation "org.opencontainers.image.title=Hello WAR" \
  --annotation "org.opencontainers.image.version=1.0.0" \
  /path/to/your/ROOT.war:application/java-archive
```

`target.clear-parent=true` tells orascout to `rm -rf` the contents of the
webapps directory before copying — this is how tomcat picks up the new WAR
cleanly without leaving an exploded copy of the old one behind.

**Static `dist/`:**

Push the dist directory as individual files (one layer per file). The
`source.dir` annotation says where in the pulled artifact the dist tree
lives.

```bash
cd /path/to/your/built/frontend                # the parent of dist/
oras push --disable-path-validation \
  "${REG}/hello-dist:latest" \
  --artifact-type application/vnd.dev.orascout.artifact.v1+json \
  --annotation "dev.orascout.v1.type=static" \
  --annotation "dev.orascout.v1.source.dir=dist" \
  --annotation "dev.orascout.v1.target.path=/var/www/html/hello-ui" \
  --annotation "dev.orascout.v1.target.clear=true" \
  --annotation "dev.orascout.v1.target.mode=0755" \
  --annotation "dev.orascout.v1.target.owner=www-data:www-data" \
  --annotation "org.opencontainers.image.title=Hello UI" \
  --annotation "org.opencontainers.image.version=1.0.0" \
  $(find dist -type f | awk '{print $0":application/octet-stream"}')
```

That `$(find …)` trick passes every file in `dist/` as a separate file
argument to `oras push`, preserving paths. For most front-end builds with a
few dozen files this works cleanly. For very large dists you may prefer to
tar+gzip and use `type=hook-only` with an extract script — but `static`
covers the common case.

### Step 6 — start the timer

```bash
sudo systemctl enable --now orascout.timer
```

The timer fires 30s after enable, then every 5 minutes. Tail the live
journal:

```bash
sudo journalctl -u orascout.service -f
```

You should see the same INFO-level "new artifact (first deploy)" lines
you saw in Part A, then "deployed". Verify your services restarted:

```bash
systemctl status hello-jar.service hello-war.service
ls -la /home/tomcat/jar-services/hello.jar
ls -la /var/www/html/hello-ui/
```

### Step 7 — push v2 and watch auto-deploy

Repeat any of the `oras push` commands above with the **same tag**
(`:latest`) but a different artifact file. The new push has a new digest
because the content changed.

Within one poll cycle (default 5m, or whatever you set) orascout will
detect the change and redeploy. Logs:

```
sudo journalctl -u orascout.service -f
```

For faster feedback while you're iterating, drop `poll_interval` in
`config.yaml` to `30s`, reload the timer (`sudo systemctl restart orascout.timer`),
and push as often as you like.

### Step 8 — wire it into CI

Your CI's "deploy" step is just `oras push`. Examples:

* **GitHub Actions:**

  ```yaml
  - uses: oras-project/setup-oras@v1
  - run: |
      oras push ghcr.io/${{ github.repository }}/hello-jar:latest \
        --artifact-type application/vnd.dev.orascout.artifact.v1+json \
        --annotation "dev.orascout.v1.type=jar" \
        --annotation "dev.orascout.v1.source.file=hello.jar" \
        --annotation "dev.orascout.v1.target.path=/home/tomcat/jar-services/hello.jar" \
        --annotation "dev.orascout.v1.service.name=hello-jar.service" \
        --annotation "org.opencontainers.image.title=Hello JAR" \
        --annotation "org.opencontainers.image.version=${{ github.sha }}" \
        target/hello.jar:application/java-archive
  ```

* **GitLab CI** is virtually identical — install the oras CLI, run the
  same command.

That's the whole CD pipeline. No SSH from CI to the target host, no
deployment agent over the network, no docker registry token sitting on the
build runner. Push goes to the registry; the target pulls.

---

## What to read next

* [SPEC.md](SPEC.md) — full reference of every annotation key and its
  effect. Skim this once before you push anything weird.
* [examples/config.yaml](examples/config.yaml) — annotated production config.
* [test/README.md](test/README.md) — how to run the test suite locally.

If you run into anything that the [SPEC](SPEC.md) doesn't cover, that's a
bug — please open an issue with the manifest you tried to push and the
behaviour you expected.
