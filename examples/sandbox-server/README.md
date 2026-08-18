# sandbox-server

A "fake Linux server" Docker container that mirrors a typical production
stack — exactly the paths and service-name conventions you'd see on a real
host — so you can rehearse the orascout install and connect it to your real
registry before touching a real machine.

## How this differs from `linux-server-demo`

| | `linux-server-demo` | `sandbox-server` (this one) |
|---|---|---|
| **Purpose** | TUTORIAL.md Part B — quick demo of the systemd path. | Personal rehearsal rig for a production install. |
| **orascout** | Pre-installed and pre-configured. | NOT installed. You install it by hand — `curl` the binary from the GitHub release, then `cp` systemd units from `/staging/`. |
| **Registry** | Local `registry:2` in the same compose stack. | None — you point at YOUR real registry (Docker Hub, GHCR, Harbor). |
| **Repos** | Demo names (`hello-jar`, `hello-war`, `hello-dist`). | Generic placeholders (`foo`, `bar`, `bar-ui`) — swap in your own. |
| **Paths** | Demo-friendly (`/opt/orascout/jars`, `/var/www/html`). | Production-style conventions: `/home/tomcat/jar-services/`, `/opt/tomcat/instances/Bar/`, `/var/www/html/Bar-UI/`. |

If you just want to *see* orascout work end-to-end, use `linux-server-demo`.
If you want to **rehearse the install** on a host that looks like a real
target, use this one.

## What's inside the container

| Path                                                  | What it is                                 |
|-------------------------------------------------------|--------------------------------------------|
| `/opt/tomcat/`                                        | Apache Tomcat 8.5.100 (CATALINA_HOME)      |
| `/opt/tomcat/instances/Bar/`                          | Tomcat instance (CATALINA_BASE)            |
| `/opt/tomcat/instances/Bar/webapps/ROOT.war`          | Placeholder WAR                            |
| `/home/tomcat/jar-services/foo.jar`                   | Placeholder JAR (real running HTTP server) |
| `/var/www/html/Bar-UI/index.html`                     | Placeholder static page                    |
| `/etc/systemd/system/foo.service`                     | systemd unit for the JAR                   |
| `/etc/systemd/system/bar.service`                     | systemd unit for the Tomcat instance       |
| `/etc/nginx/sites-available/default`                  | nginx config: serves `/var/www/html/Bar-UI/` at `/Bar-UI/` |
| `/staging/systemd/orascout.{service,timer}`           | systemd units, NOT yet installed           |
| `/staging/config.yaml.example`                        | template config, NOT yet installed         |
| `/staging/env.example`                                | template env file, NOT yet installed       |

## Quick start

```powershell
docker compose -f examples/sandbox-server/docker-compose.yml up --build -d
```

First build is ~2-4 minutes (Tomcat tarball, JDK 8, systemd-ubuntu — orascout
itself is downloaded later, in INSTALL-STEPS.md Step 4a). Then follow
[INSTALL-STEPS.md](INSTALL-STEPS.md).

## Ports exposed to the host

| Container port | Host port | Service                                |
|----------------|-----------|----------------------------------------|
| 80             | 8080      | nginx (frontend, at `/Bar-UI/`)        |
| 8080           | 8081      | Tomcat (bar WAR, at `/`)               |
| 8090           | 8090      | foo JAR HTTP server                    |

## Cleanup

```bash
docker compose -f examples/sandbox-server/docker-compose.yml down -v
```
