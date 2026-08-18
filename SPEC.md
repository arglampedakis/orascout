# orascout — Specification

Version: `v1` (this document)
Status: Draft

This document defines the contract between **push-side** tools (CI pipelines that publish
ORAS artifacts to a registry) and **pull-side** tools (the `orascout` daemon that watches
the registry and deploys artifacts on target hosts).

The contract has two halves:

1. **Manifest annotations** — keys attached to the OCI manifest at push time that tell
   the puller how to deploy the artifact.
2. **Artifact layout** — file/directory conventions inside the artifact that the
   deploy strategies expect.

A puller MUST NOT need any per-artifact configuration beyond what is described in this
spec. Everything that varies per service is carried on the manifest itself.

---

## 1. Annotation namespace

All orascout-specific annotations live under the reverse-DNS namespace
`dev.orascout.v1.*`. The `v1` segment is part of the contract — when a future,
incompatible revision of this spec is published, it will use a new prefix
(e.g. `dev.orascout.v2.*`). A puller MUST ignore any annotation whose namespace
it does not recognise.

Standard OCI annotations from the
[image-spec](https://github.com/opencontainers/image-spec/blob/main/annotations.md)
are also honored where they make sense:

| Annotation                              | Meaning to orascout                                  |
|-----------------------------------------|------------------------------------------------------|
| `org.opencontainers.image.title`        | Human-readable artifact title (used in logs).        |
| `org.opencontainers.image.description`  | Free-form description.                               |
| `org.opencontainers.image.version`      | Artifact version (informational).                    |
| `org.opencontainers.image.created`      | RFC 3339 build timestamp (informational).            |
| `org.opencontainers.image.source`       | Source repo URL (informational).                     |
| `org.opencontainers.image.revision`     | Source commit (informational).                       |

---

## 2. The `type` key

```
dev.orascout.v1.type = jar | war | static | run-once | hook-only
```

This is the **only required orascout annotation**. It selects which built-in deploy
strategy the puller will use. If absent or set to a value the puller doesn't
recognise, the artifact is skipped with a logged error (digest state is NOT updated,
so the puller will retry on the next cycle).

| Type        | What it does                                                                |
|-------------|-----------------------------------------------------------------------------|
| `jar`       | Stop service, copy a `.jar` to a target path, start service.                |
| `war`       | Stop service, clear webapps dir, copy `.war`, start service.                |
| `static`    | Sync a directory of static files into a destination (e.g. nginx webroot).   |
| `run-once`  | Run a binary/jar to completion (e.g. DB migration). No service lifecycle.   |
| `hook-only` | Run only the user-supplied pre/post hook scripts. No built-in copy/restart. |

---

## 3. Per-strategy annotations

### 3.1 `type = jar`

| Annotation                                | Required | Example                                    |
|-------------------------------------------|----------|--------------------------------------------|
| `dev.orascout.v1.source.file`             | yes      | `foo.jar`                          |
| `dev.orascout.v1.target.path`             | yes      | `/home/tomcat/jar-services/foo.jar`|
| `dev.orascout.v1.service.name`            | yes      | `foo.service`                      |
| `dev.orascout.v1.service.manager`         | no       | `systemd-user` (default) \| `systemd` \| `none` |
| `dev.orascout.v1.target.mode`             | no       | `0644`                                     |
| `dev.orascout.v1.target.owner`            | no       | `tomcat:tomcat`                            |

Behaviour: `systemctl [--user] stop <svc>` → `cp <artifact>/<source.file> <target.path>`
→ apply `mode`/`owner` if set → `systemctl [--user] start <svc>`.

### 3.2 `type = war`

| Annotation                                | Required | Example                                              |
|-------------------------------------------|----------|------------------------------------------------------|
| `dev.orascout.v1.source.file`             | yes      | `ROOT.war`                                           |
| `dev.orascout.v1.target.path`             | yes      | `/opt/tomcat/instances/foo/webapps/ROOT.war`         |
| `dev.orascout.v1.service.name`            | yes      | `foo.service`                                        |
| `dev.orascout.v1.service.manager`         | no       | `systemd-user` (default) \| `systemd` \| `none`      |
| `dev.orascout.v1.target.clear-parent`     | no       | `true` (default) — `rm -rf <parent>/*` before copy.  |

Behaviour: `systemctl stop` → optionally clear parent dir → copy war → `systemctl start`.

### 3.3 `type = static`

| Annotation                                | Required | Example                              |
|-------------------------------------------|----------|--------------------------------------|
| `dev.orascout.v1.source.dir`              | yes      | `dist`                               |
| `dev.orascout.v1.target.path`             | yes      | `/var/www/html/foo`                  |
| `dev.orascout.v1.target.clear`            | no       | `true` (default)                     |
| `dev.orascout.v1.target.mode`             | no       | `0755`                               |
| `dev.orascout.v1.target.owner`            | no       | `www-data:www-data`                  |

Behaviour: optionally clear target → copy contents of `<artifact>/<source.dir>/*` into
`<target.path>` → apply mode/owner recursively.

### 3.4 `type = run-once`

| Annotation                                | Required | Example                              |
|-------------------------------------------|----------|--------------------------------------|
| `dev.orascout.v1.source.file`             | yes      | `db-migration.jar`                   |
| `dev.orascout.v1.runonce.command`         | yes      | `java -jar -Dfile.encoding=UTF-8 {file}` |

`{file}` in the command template is replaced with the absolute path to the
pulled source file. The command is run to completion in the artifact directory.
A non-zero exit code marks the deploy as failed (digest state NOT updated).

### 3.5 `type = hook-only`

No additional required annotations. The artifact must ship at least one hook
script (see §4). Use this when none of the built-in strategies fit and you want
to script the deployment yourself entirely.

---

## 4. Hook scripts

Two optional hook annotations work with **any** strategy:

| Annotation                       | Example                       |
|----------------------------------|-------------------------------|
| `dev.orascout.v1.hook.pre`       | `scripts/pre-deploy.sh`       |
| `dev.orascout.v1.hook.post`      | `scripts/post-deploy.sh`      |

The value is a path **relative to the root of the pulled artifact**. The script
MUST be executable (mode bit set in the artifact). Hooks receive these env vars:

| Env var                  | Meaning                                                       |
|--------------------------|---------------------------------------------------------------|
| `ORASCOUT_ARTIFACT_DIR`  | Absolute path where the artifact was pulled.                  |
| `ORASCOUT_REPO`          | `repo:tag` that triggered the deploy.                         |
| `ORASCOUT_DIGEST`        | New digest (sha256:...).                                      |
| `ORASCOUT_PREV_DIGEST`   | Previous digest, or empty on first deploy.                    |
| `ORASCOUT_TYPE`          | Value of `dev.orascout.v1.type`.                              |
| `ORASCOUT_PHASE`         | `pre` or `post`.                                              |

A non-zero exit from `hook.pre` aborts the deploy (state NOT updated). A non-zero
exit from `hook.post` is logged as a warning but does NOT roll back; the deploy is
considered successful and state IS updated.

---

## 5. Healthcheck (optional)

| Annotation                                | Example                                      |
|-------------------------------------------|----------------------------------------------|
| `dev.orascout.v1.healthcheck.cmd`         | `curl -fsS http://127.0.0.1:8080/health`     |
| `dev.orascout.v1.healthcheck.timeout`     | `60s` (default `30s`)                        |
| `dev.orascout.v1.healthcheck.interval`    | `2s`  (default `2s`)                         |

If `healthcheck.cmd` is set, the puller will run it repeatedly after the deploy
finishes until it exits 0 (success) or the timeout elapses (failure). On failure
the deploy is marked failed and digest state is NOT updated.

---

## 6. Artifact layout conventions

The puller pulls all blobs in the artifact into a single directory (one directory
per repo, configurable). Beyond that, the only conventions are:

* Files referenced by `source.file` / `source.dir` are resolved relative to the
  pull directory.
* Hook scripts referenced by `hook.pre` / `hook.post` are resolved the same way.

There is no required top-level layout — pushers may ship a flat set of files or
nest them in subdirectories, so long as the annotations point at the right paths.

---

## 7. Pushing artifacts (the contract for CI)

A push using the `oras` CLI looks like:

```bash
oras push docker.io/myorg/my-service:latest \
  --artifact-type application/vnd.dev.orascout.artifact.v1+json \
  --annotation "dev.orascout.v1.type=jar" \
  --annotation "dev.orascout.v1.source.file=foo.jar" \
  --annotation "dev.orascout.v1.target.path=/home/tomcat/jar-services/foo.jar" \
  --annotation "dev.orascout.v1.service.name=foo.service" \
  --annotation "org.opencontainers.image.title=Foo" \
  --annotation "org.opencontainers.image.version=1.4.2" \
  foo.jar:application/vnd.java.archive
```

The `--artifact-type` is a hint; the puller does NOT enforce it. The puller
inspects manifest annotations, not the media type.

---

## 8. State file

The puller maintains a JSON state file (path is config-driven, default
`/var/lib/orascout/state.json`):

```json
{
  "schema": 1,
  "entries": {
    "docker.io/myorg/foo:latest": {
      "digest": "sha256:abc...",
      "deployedAt": "2026-05-25T10:30:00Z",
      "type": "jar"
    }
  }
}
```

State is updated **only after a successful deploy** (strategy succeeded AND
healthcheck passed if configured). On failure the state is left untouched so
the next cycle will retry.

---

## 9. Concurrency

The puller takes a file lock at startup (default `/var/run/orascout.lock`) to
prevent overlapping cycles (e.g. from a systemd timer firing while a previous
run is still pulling a large artifact). A failed lock acquisition is logged at
INFO level and the process exits 0 — this is normal, not an error.

---

## 10. Forward compatibility

* Unknown annotations under `dev.orascout.v1.*` MUST be ignored.
* Unknown annotations under any other namespace MUST be ignored.
* A future `dev.orascout.v2.*` namespace will be introduced for breaking
  changes; both v1 and v2 annotations MAY co-exist on the same manifest during
  a migration. The puller will prefer the highest version it understands.
