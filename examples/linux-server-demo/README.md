# linux-server-demo

**TUTORIAL.md Part B** lives here — a faithful simulation of a real Linux
server + systemd in a single privileged container, no VM required. Use it
to verify orascout's real-systemd path end-to-end before deploying to an
actual host (which is [TUTORIAL.md Part C](../../TUTORIAL.md#part-c--real-linux-server)).

> **Warning:** this stack uses `privileged: true` because systemd inside a
> container can't manage cgroups otherwise. That's fine for a local test
> rig but **never appropriate for production**. The real systemd-host setup
> in [TUTORIAL.md Part C](../../TUTORIAL.md#part-c--real-linux-server) does
> not need privileged mode.

## What's inside

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
│  ┌──────────────────────────────────────────┐  │
│  │ /opt/orascout/jars/hello.jar             │  │ → hello-jar.service
│  │ /opt/tomcat/webapps/ROOT.war             │  │ → tomcat.service
│  │ /var/www/html/index.html                 │  │ → nginx (no restart)
│  └──────────────────────────────────────────┘  │
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
│ (compose net)  │  pushes  │ (one-shot, JDK 8 │
│                │          │  builds real JAR │
│                │          │  + WAR fixtures) │
└────────────────┘          └──────────────────┘
```

The `pusher` container compiles **real Java 8 / Tomcat 8.5 fixtures** at
image-build time:

* `hello-v1.jar` / `hello-v2.jar` — compiled from
  [pusher/src/HelloServer.java](pusher/src/HelloServer.java), a tiny
  HTTP server on `:8082` whose `VERSION` constant differs between
  builds.
* `ROOT-v1.war` / `ROOT-v2.war` — minimal valid WAR files with a real
  `WEB-INF/web.xml` and an `index.html` that displays the version.
* `dist/v1` and `dist/v2` — static HTML for the nginx target.

So you can actually `curl` (or browse) each of the three services and
**see the version change** after each push, instead of just inspecting
file contents.

## Prereqs

- Docker Desktop on Windows / macOS, or Docker Engine on Linux. That's it.

## Step 1 — bring up the stack

From the orascout repo root:

```powershell
docker compose -f examples/linux-server-demo/docker-compose.yml up --build -d
```

First-time build takes ~3-5 minutes (downloads `golang`, `eclipse-temurin:8-jdk`,
`registry:2`, `jrei/systemd-ubuntu:22.04`, Tomcat 8.5.100, alpine + oras CLI;
compiles the JAR/WAR fixtures; compiles orascout). Subsequent runs reuse
cached layers and start in seconds.

When `up` returns, `docker compose ps` should show:

```
NAME                           STATUS
orascout-lsd-registry          Up (healthy)
orascout-lsd-pusher            Exited (0)          ← one-shot, has finished
orascout-lsd-linux-server      Up (healthy)
```

## Step 2 — watch the first deploy cycle

orascout's first cycle fires 15s after the linux-server container boots,
then every 30s. Stream the systemd journal for orascout:

```bash
docker compose -f examples/linux-server-demo/docker-compose.yml \
  exec linux-server journalctl -u orascout.service -f
```

You should see lines like:

```
orascout[42]: msg="new artifact (first deploy)" ref=registry:5000/orascout-demo/hello-jar:latest
orascout[42]: msg="stopping service" unit=hello-jar.service manager=systemd
orascout[42]: msg="copying jar" src=.../hello.jar dst=/opt/orascout/jars/hello.jar
orascout[42]: msg="starting service" unit=hello-jar.service
orascout[42]: msg=deployed type=jar

orascout[42]: msg="new artifact (first deploy)" ref=registry:5000/orascout-demo/hello-war:latest
orascout[42]: msg="stopping service" unit=tomcat.service manager=systemd
orascout[42]: msg="clearing webapps dir" dir=/opt/tomcat/webapps
orascout[42]: msg="copying war" src=.../ROOT.war dst=/opt/tomcat/webapps/ROOT.war
orascout[42]: msg="starting service" unit=tomcat.service
orascout[42]: msg=deployed type=war

orascout[42]: msg="new artifact (first deploy)" ref=registry:5000/orascout-demo/hello-dist:latest
orascout[42]: msg="syncing static directory" dst=/var/www/html clear=true
orascout[42]: msg=deployed type=static
```

`Ctrl+C` to stop tailing.

## Step 3 — verify each service is live

All three should now serve their v1 content:

```bash
curl http://localhost:8082         # JAR HTTP server
# -> Hello from JAR v1

curl http://localhost:8081         # Tomcat-served WAR
# -> <h1>Hello from v1 (WAR)</h1>   (with HTML chrome)

curl http://localhost:8080         # nginx-served static
# -> <h1>Hello from v1 (static)</h1>
```

Browser views: open all three URLs to see the colors / styling.

You can also poke at systemd directly:

```bash
docker compose -f examples/linux-server-demo/docker-compose.yml exec linux-server \
  systemctl status tomcat.service hello-jar.service orascout.timer
```

## Step 4 — push v2 and watch the live redeploy

```bash
docker compose -f examples/linux-server-demo/docker-compose.yml run --rm pusher v2
```

Within 30 seconds the orascout timer fires, the journal shows three
"artifact changed" entries followed by three "deployed" lines, and:

```bash
curl http://localhost:8082         # -> Hello from JAR v2
curl http://localhost:8081         # -> Hello from v2 (WAR)
curl http://localhost:8080         # -> Hello from v2 (static)
```

The JAR service's HTTP response changes because orascout actually
`systemctl stop`-ped it, replaced `/opt/orascout/jars/hello.jar`, and
`systemctl start`-ed it. Same story for Tomcat. The static deploy needs
no service restart at all — nginx just serves whatever's in
`/var/www/html`.

## Step 5 — push v1 again to confirm rollback works

```bash
docker compose -f examples/linux-server-demo/docker-compose.yml run --rm pusher v1
```

Same loop, same observable change in the other direction. This is the
exact mechanism you'd use to roll back a bad deploy: re-push the previous
artifact and orascout treats it as new (different digest from the
currently-deployed one).

## Step 6 — tear down

```bash
docker compose -f examples/linux-server-demo/docker-compose.yml down -v
```

`-v` removes the volumes so the next `up` starts from a clean slate.

## Mapping this back to a real Linux server

Everything in [linux-server/](linux-server/) maps 1:1 to files you'd put
on a real host:

| In the demo container                                  | On a real host                                                   |
|--------------------------------------------------------|------------------------------------------------------------------|
| `/etc/orascout/config.yaml` (from `orascout-config.yaml`) | `/etc/orascout/config.yaml`                                       |
| `/etc/systemd/system/orascout.service` + `.timer`      | Same path — copy from [packaging/systemd/](../../packaging/systemd/) |
| `/etc/systemd/system/hello-jar.service`                | Same path — yours to write, matching your service                |
| `/etc/systemd/system/tomcat.service`                   | Same path — typically installed by your distro or by hand        |
| `/etc/nginx/sites-available/default`                   | Same path on Debian/Ubuntu; `/etc/nginx/conf.d/*.conf` on RHEL   |
| `/usr/local/bin/orascout`                              | Same path — from `install.sh` or a manual `scp`                  |

So once this demo runs cleanly, the **same files** drive a real production
host. The only thing that changes is the `registry_prefix` in
`orascout-config.yaml` (point at Docker Hub / GHCR / your private
registry) and the registry credentials (via `/etc/orascout/env`).

## Troubleshooting

* **`Error response from daemon: cgroup` on `docker compose up`:** older
  Docker Engine (< 25) doesn't recognise `cgroup: host`. Remove that
  one line from `docker-compose.yml` and try again.
* **Health stays `starting` forever:** systemd in container can be slow
  to settle. `docker compose logs linux-server` shows boot progress;
  give it 60s. If it never goes healthy, run
  `docker compose exec linux-server systemctl list-units --failed` to
  see what unit fell over.
* **Tomcat won't start:** check `journalctl -u tomcat.service` for the
  catalina.out output. Usually a JVM not-found error means
  `JAVA_HOME` in `tomcat.service` doesn't match the installed JDK.
* **First-build is huge** (~1 GB image size for linux-server). Most of
  it is the Tomcat tarball + JDK 8 + systemd-ubuntu base. Cached
  builds are much smaller incremental layers.
