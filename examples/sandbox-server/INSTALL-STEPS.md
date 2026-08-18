# Install orascout on the sandbox server — step by step

This walks you through the exact actions you'd perform on a real Linux server,
just inside a sandbox container that mirrors your production paths. By the end
you will have:

* orascout installed and running on a 30s systemd timer
* Pointed at **your** real registry (Docker Hub, GHCR, Harbor — your call)
* Three repos connected: `foo`, `bar`, `bar-ui`
* The placeholder content replaced by your real artifacts after each `oras push`

Time required: ~5 minutes after the image build finishes.

---

## Prereqs

* Docker Desktop running.
* An account on a registry you can push to. Most common:
  * **Docker Hub** — create a private repo and a [Personal Access Token](https://hub.docker.com/settings/security).
  * **GitHub Container Registry (GHCR)** — a fine-grained PAT with
    `read:packages` + `write:packages` scopes.
* Your three real artifacts ready to push (or use the demo artifacts you
  built for `linux-server-demo` — the strategies don't care about content).

If you don't have artifacts handy yet, the install still works; orascout will
just keep saying "no change" for each repo until you push something.

---

## Step 1 — bring up the sandbox

From the orascout repo root:

```powershell
docker compose -f examples/sandbox-server/docker-compose.yml up --build -d
```

Wait for the build (~3-5 minutes first time, cached thereafter). When it's
done:

```powershell
docker compose -f examples/sandbox-server/docker-compose.yml ps
```

`STATUS` should be `Up (healthy)` (or `(health: starting)` for the first ~30s
while Tomcat boots).

## Step 2 — confirm the placeholder services are running

These all hit **placeholders** that ship inside the image. Replacing them is
what proves orascout is working later.

```bash
curl http://localhost:8090
# -> foo placeholder — orascout has not deployed a real version yet...

curl http://localhost:8081/
# -> <h1>bar WAR — placeholder</h1>... (HTML)

curl http://localhost:8080/Bar-UI/
# -> <h1>bar-ui — placeholder</h1>... (HTML)
```

If any of those don't respond, see [Troubleshooting](#troubleshooting) at the
bottom.

## Step 3 — drop into the container as root

```powershell
docker compose -f examples/sandbox-server/docker-compose.yml exec sandbox bash
```

Your prompt is now inside the container. Everything from here through Step 7
runs in this shell.

> **Git Bash on Windows:** if `docker exec` mangles paths, prefix with
> `MSYS_NO_PATHCONV=1` or use PowerShell / cmd for these commands.

## Step 4 — install orascout

Three actions: download the binary from the GitHub release, copy the
systemd units, reload systemd. This is identical to what you'd run on a
real Ubuntu host — that's the whole point of the sandbox.

### 4a — download the binary

How you fetch it depends on whether the repo is public or still private.

#### If the repo is **public**

One-line curl, no auth:

```bash
curl -fsSL -L \
  https://github.com/arglampedakis/orascout/releases/latest/download/orascout-linux-amd64 \
  -o /usr/local/bin/orascout
chmod +x /usr/local/bin/orascout
orascout version
# -> orascout v0.1.0
```

#### If the repo is **private**

The `releases/latest/download/...` URL is finicky on private repos and
sometimes 404s even with valid auth. The reliable path is via the GitHub
API: look up the asset ID for the release, then download that asset with an
`Accept: application/octet-stream` header.

Create a PAT first if you don't have one:
* Classic PAT: <https://github.com/settings/tokens> → `repo` scope.
* Fine-grained PAT: <https://github.com/settings/personal-access-tokens/new>
  → scope to the `arglampedakis/orascout` repo → `Contents: Read-only`.

Then, inside the sandbox container:

```bash
# 1. paste the PAT (stays in shell memory only)
read -sp "GitHub PAT: " GITHUB_PAT; echo

# 2. look up the asset's API ID for the v0.1.0 release
ASSET_ID=$(curl -fsSL \
  -H "Authorization: Bearer $GITHUB_PAT" \
  -H "Accept: application/vnd.github+json" \
  https://api.github.com/repos/arglampedakis/orascout/releases/tags/v0.1.0 \
  | jq -r '.assets[] | select(.name=="orascout-linux-amd64") | .id')
echo "asset id: $ASSET_ID"

# 3. download the asset bytes
curl -fsSL -L \
  -H "Authorization: Bearer $GITHUB_PAT" \
  -H "Accept: application/octet-stream" \
  "https://api.github.com/repos/arglampedakis/orascout/releases/assets/$ASSET_ID" \
  -o /usr/local/bin/orascout

chmod +x /usr/local/bin/orascout
orascout version
# -> orascout v0.1.0
```

`curl -L` follows the redirect from `api.github.com` to the CDN that
actually serves the asset; the GitHub auth header is stripped on the
cross-host hop (curl default), so the CDN sees only its own signed token.

If `ASSET_ID` comes back empty, the release isn't published yet — see
[Step 0 of CONTRIBUTING-style notes](#troubleshooting) at the bottom.

### 4b — install the systemd units

These ship inside the sandbox at `/staging/systemd/` because they're project
files that live in the repo, not release artifacts. On a real host you'd
`scp` them from your local checkout or `curl` them from
`raw.githubusercontent.com`.

```bash
cp /staging/systemd/orascout.service /etc/systemd/system/orascout.service
cp /staging/systemd/orascout.timer   /etc/systemd/system/orascout.timer
systemctl daemon-reload
```

### 4c — create the state + config directories

```bash
mkdir -p /etc/orascout /var/lib/orascout /var/log/orascout
```

## Step 5 — configure your registry

Two files: `/etc/orascout/config.yaml` (what to watch) and
`/etc/orascout/env` (credentials, kept out of the main config).

```bash
cp /staging/config.yaml.example /etc/orascout/config.yaml
cp /staging/env.example         /etc/orascout/env
chmod 600 /etc/orascout/env
```

### Edit the config

```bash
nano /etc/orascout/config.yaml
```

Change **one** line — the `registry_prefix`. Replace `docker.io/CHANGE_ME_TO_YOUR_ORG`
with your actual registry + namespace. Examples:

* Docker Hub user `myorg`: `docker.io/myorg`
* GHCR user `aglampedakis`:      `ghcr.io/aglampedakis`
* Self-hosted Harbor:            `harbor.example.com/myproject`

The three `repos:` entries (`foo`, `bar`, `bar-ui`)
already match the systemd setup baked into this container, so leave them
alone unless your repo names are different.

> **Don't change `service.manager`, `target.path`, etc. here** — those live on
> the manifest, not in the config. The config only says **which** repos to
> watch.

Save: `Ctrl-O`, `Enter`, `Ctrl-X`.

### Edit the env file

```bash
nano /etc/orascout/env
```

Fill in your username and the Personal Access Token (PAT) you generated:

```
REGISTRY_USERNAME=myorg
REGISTRY_TOKEN=dckr_pat_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

> **Always use a PAT, never your account password.** Docker Hub:
> *Account Settings → Security → New Access Token*. GHCR: a fine-grained
> Personal Access Token with `read:packages` (and `write:packages` if your
> pushes also go from this host). Harbor: a robot account credential.
>
> The PAT goes in the basic-auth password slot under the hood — that's
> just how Docker Hub and GHCR work. The variable is called `REGISTRY_TOKEN`
> to reflect what you actually put there, but in `config.yaml` it shows up
> as `password: $REGISTRY_TOKEN` because that's what the registry HTTP
> protocol expects. If your registry instead issues real bearer tokens (some
> Harbor setups), see the commented note in `config.yaml`.

Save and exit.

## Step 6 — start orascout

```bash
systemctl enable --now orascout.timer
systemctl status orascout.timer
```

`Active: active (waiting)` confirms it's running. The timer fires 15 seconds
after enable, then every 30 seconds.

Tail the log to watch the first cycle:

```bash
journalctl -u orascout.service -f
```

If the repos in the registry don't have any manifests yet, you'll see one
INFO log per repo saying it could not resolve the manifest digest and the
cycle moving on — that's normal until you push something. (See Step 7.)

Press `Ctrl-C` to stop tailing. Leave this shell open in another tab if you
want to keep watching.

## Step 7 — push your real artifacts

This part runs **outside** the container, from your laptop. We'll use a
one-shot Docker container with the `oras` CLI pre-installed, so you don't
need to install anything on your host. (If you already have the oras CLI on
your machine, you can drop the `docker run ...` wrapper.)

Set these once for convenience (PowerShell shown — adapt for bash):

```powershell
$env:REG       = "docker.io/myorg"        # match your config.yaml
$env:REG_USR   = "myorg"
$env:REG_TOKEN = "dckr_pat_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
```

> `oras push` uses `--password` as the flag name for historical reasons; the
> value you supply there is your PAT, same one that lives in `REGISTRY_TOKEN`
> inside the container. The `--username` / `--password` pair works without
> an explicit `oras login` first, which is convenient for one-off pushes.

### 7a — push the JAR

You need a real `foo.jar` on your host (or anything — the strategy
just copies bytes). Place it in your current directory.

```powershell
docker run --rm -v "${PWD}:/work" -w /work bitnami/oras:1.2.0 `
  push --username $env:REG_USR --password $env:REG_TOKEN `
    "${env:REG}/foo:latest" `
    --artifact-type application/vnd.dev.orascout.artifact.v1+json `
    --annotation "dev.orascout.v1.type=jar" `
    --annotation "dev.orascout.v1.source.file=foo.jar" `
    --annotation "dev.orascout.v1.target.path=/home/tomcat/jar-services/foo.jar" `
    --annotation "dev.orascout.v1.service.name=foo.service" `
    --annotation "dev.orascout.v1.service.manager=systemd" `
    --annotation "dev.orascout.v1.target.owner=tomcat:tomcat" `
    --annotation "org.opencontainers.image.title=foo" `
    --annotation "org.opencontainers.image.version=1.0.0" `
    foo.jar:application/java-archive
```

### 7b — push the WAR

Same idea, with the WAR-specific annotations:

```powershell
docker run --rm -v "${PWD}:/work" -w /work bitnami/oras:1.2.0 `
  push --username $env:REG_USR --password $env:REG_TOKEN `
    "${env:REG}/bar:latest" `
    --artifact-type application/vnd.dev.orascout.artifact.v1+json `
    --annotation "dev.orascout.v1.type=war" `
    --annotation "dev.orascout.v1.source.file=ROOT.war" `
    --annotation "dev.orascout.v1.target.path=/opt/tomcat/instances/Bar/webapps/ROOT.war" `
    --annotation "dev.orascout.v1.target.clear-parent=true" `
    --annotation "dev.orascout.v1.service.name=bar.service" `
    --annotation "dev.orascout.v1.service.manager=systemd" `
    --annotation "dev.orascout.v1.target.owner=tomcat:tomcat" `
    --annotation "org.opencontainers.image.title=bar" `
    --annotation "org.opencontainers.image.version=1.0.0" `
    ROOT.war:application/java-archive
```

`target.clear-parent=true` matches your existing bash script's `rm -rf
$WARS_DIR/.../webapps/*` step — Tomcat reads the new WAR cleanly without an
old exploded copy fighting it.

### 7c — push the static `dist/`

Push the contents of your built frontend's `dist/` directory. From a
directory that has `dist/` as a subfolder:

```powershell
$files = Get-ChildItem -Path dist -Recurse -File |
  ForEach-Object { $_.FullName.Substring((Get-Location).Path.Length + 1).Replace('\','/') + ':application/octet-stream' }

docker run --rm -v "${PWD}:/work" -w /work bitnami/oras:1.2.0 `
  push --plain-http=false --disable-path-validation `
    --username $env:REG_USR --password $env:REG_TOKEN `
    "${env:REG}/bar-ui:latest" `
    --artifact-type application/vnd.dev.orascout.artifact.v1+json `
    --annotation "dev.orascout.v1.type=static" `
    --annotation "dev.orascout.v1.source.dir=dist" `
    --annotation "dev.orascout.v1.target.path=/var/www/html/Bar-UI" `
    --annotation "dev.orascout.v1.target.clear=true" `
    --annotation "dev.orascout.v1.target.mode=0755" `
    --annotation "org.opencontainers.image.title=bar-ui" `
    --annotation "org.opencontainers.image.version=1.0.0" `
    $files
```

The `$files` line passes every file under `dist/` as a separate layer so the
paths are preserved on pull — orascout's `static` strategy then has a real
`dist/` directory inside the pulled artifact to sync from.

## Step 8 — watch the deploys land

Inside the container:

```bash
journalctl -u orascout.service -f
```

Within 30 seconds you'll see the three `msg=deployed` lines you saw in the
linux-server-demo, this time for `foo`, `bar`, and
`bar-ui`. Once each appears:

```bash
# from your host (or the container — both work)
curl http://localhost:8090           # NOT the placeholder anymore — your real foo
curl http://localhost:8081/          # your real bar
curl http://localhost:8080/Bar-UI/   # your real UI
```

If your real JAR doesn't listen on `:8090`, the curl will just hang/refuse;
the deploy still happened. Check `systemctl status foo.service` to
see what your real service actually did on start.

## Step 9 — push v2 and watch the auto-redeploy

This is the whole point: re-push the **same tag** with new content. orascout
diffs the manifest digest against its state file, sees a change, and
redeploys.

Just rerun any of the pushes from Step 7 with a new artifact file (a new
build of your JAR/WAR/dist). Within 30 seconds, the journal shows
"artifact changed" → "stopping service" → "copying" → "starting service" →
"deployed". The HTTP responses change to match the new content.

This is exactly what your CI pipeline will do: build → `oras push` → done.
The host pulls, no SSH from CI to host, no deploy agent over the network.

---

## What you've just rehearsed

Every command in Step 4 (binary copy, unit copy, daemon-reload) and Step 5
(write `config.yaml`, write `env`, chmod) is byte-for-byte what you'd do on
a real Linux host. So is Step 6 (`systemctl enable --now`). The Step 7
pushes are exactly what your CI will run, against the same registry.

When you're ready to do this on a real Ubuntu/RHEL host:

1. Replace `cp /staging/orascout /usr/local/bin/` with
   `curl -fsSL https://github.com/arglampedakis/orascout/releases/latest/download/orascout-linux-amd64 -o /usr/local/bin/orascout`
2. Everything else is identical — same systemd units, same config, same
   annotations on the pushes.

The only meaningful difference is that on a real host you typically run
orascout as a least-privilege user instead of root. See
[TUTORIAL.md Part C](../../TUTORIAL.md#part-c--real-linux-server) for that
hardening step.

---

## Troubleshooting

* **Step 4a `curl` returns 404 on the release URL:** the v0.1.0 release
  doesn't exist on GitHub yet. Most common cause after recreating a repo:
  workflow permissions reset to "Read repository contents" (read-only), so
  goreleaser couldn't create the release. Fix at
  <https://github.com/arglampedakis/orascout/settings/actions> — set
  **Workflow permissions** to **"Read and write permissions"** and Save.
  Then go to **Actions → release.yml → "Run workflow"** and fire it
  manually. Check <https://github.com/arglampedakis/orascout/releases>
  shows the v0.1.0 release with three assets attached before retrying
  Step 4a.

* **Step 4a `ASSET_ID` comes back empty / null:** same root cause as above,
  or the asset name doesn't match. Inspect the API response:
  `curl -fsSL -H "Authorization: Bearer $GITHUB_PAT" https://api.github.com/repos/arglampedakis/orascout/releases/tags/v0.1.0 | jq .assets`.
  You should see `orascout-linux-amd64`, `orascout-linux-arm64`, and
  `checksums.txt`. If `.assets` is `[]`, the release exists but goreleaser
  failed to upload — re-run the workflow.

* **`curl http://localhost:8090` connection refused immediately after `up`:**
  Tomcat / nginx / foo take 10-30s to start under systemd. Wait,
  then `docker compose ... ps` — once it says `(healthy)` you're good.

* **`foo.service` won't start, journal says "unable to access jarfile":**
  The placeholder JAR didn't make it into the image. Rebuild without cache:
  `docker compose ... build --no-cache sandbox`.

* **orascout journal says "auth required" or "401 Unauthorized":**
  `/etc/orascout/env` has wrong creds. Edit and `systemctl restart orascout.timer`.
  PATs need the right scopes (Docker Hub: anything; GHCR: `read:packages`).

* **orascout journal says "manifest unknown" for one of the three repos:**
  That repo doesn't exist yet on your registry. Push something to it
  (Step 7); orascout will pick it up on the next cycle.

* **You want orascout to poll faster than 30s while iterating:**
  Edit `/staging/orascout.timer` before installing it, or edit
  `/etc/systemd/system/orascout.timer` and `systemctl daemon-reload && systemctl restart orascout.timer`.

* **"Stale lock" warnings in the journal:**
  A previous cycle crashed without releasing `/var/lib/orascout/orascout.lock`.
  orascout reclaims same-PID stale locks automatically; nothing to do.

* **Tomcat deploy succeeds but old WAR is still served:**
  Make sure your push included `--annotation "dev.orascout.v1.target.clear-parent=true"`
  — without it, Tomcat may keep using the old exploded ROOT directory next
  to the new WAR.
