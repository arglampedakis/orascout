#!/usr/bin/env bash
# End-to-end test for orascout.
#
# This script is registry-host-agnostic. It assumes:
#   - An OCI registry is reachable at $REGISTRY_HOST (default: localhost:5000)
#   - Go, the `oras` CLI, and `jq` are on PATH
#
# Bring up the registry separately (see test/docker-compose.yml). The script
# itself does no docker orchestration — that way it works identically whether
# it runs on the host (hybrid mode) or inside the e2e container (full-docker
# mode).
#
# Assertions:
#   1. push v1 -> orascout --once -> target file appears with v1 content
#   2. push v2 -> orascout --once -> target file updates to v2 content
#   3. orascout --once again -> NO redeploy (target mtime unchanged)

set -euo pipefail

REGISTRY_HOST="${REGISTRY_HOST:-localhost:5000}"
REPO_NS="orascout-test"
REPO_NAME="hello"
REPO_TAG="latest"
FULL_REF="${REGISTRY_HOST}/${REPO_NS}/${REPO_NAME}:${REPO_TAG}"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

TMPDIR="$(mktemp -d -t orascout-e2e.XXXXXX)"
TARGET_DIR="${TMPDIR}/target"
mkdir -p "${TARGET_DIR}"
TARGET_FILE="${TARGET_DIR}/hello.jar"

cleanup() {
  rc=$?
  echo
  if [[ $rc -eq 0 ]]; then
    rm -rf "$TMPDIR"
    echo "=== PASS ==="
  else
    echo "=== FAIL (artifacts preserved at $TMPDIR) ==="
  fi
  exit $rc
}
trap cleanup EXIT

# --- 1. wait for registry -------------------------------------------------
echo "--- waiting for registry at http://${REGISTRY_HOST} ---"
for i in $(seq 1 30); do
  if curl -fsS "http://${REGISTRY_HOST}/v2/" >/dev/null 2>&1; then
    break
  fi
  if [[ $i -eq 30 ]]; then
    echo "registry never became reachable at ${REGISTRY_HOST}"
    exit 1
  fi
  sleep 1
done

# --- 2. build orascout ---------------------------------------------------
# Ensure go.sum is up to date. Required on a fresh clone where go.sum doesn't
# exist yet; a no-op on subsequent runs. This writes go.sum (and possibly
# tweaks go.mod) on the bind-mounted source tree, which is intentional —
# commit the resulting files so CI can `go build` without an implicit tidy.
echo "--- go mod tidy ---"
go mod tidy

echo "--- building orascout ---"
go build -o "${TMPDIR}/orascout" ./cmd/orascout

# --- 3. write config -----------------------------------------------------
CONFIG="${TMPDIR}/config.yaml"
cat > "${CONFIG}" <<EOF
registry_prefix: ${REGISTRY_HOST}/${REPO_NS}
poll_interval: 60s
artifacts_dir: ${TMPDIR}/artifacts
state_file: ${TMPDIR}/state.json
lock_file: ${TMPDIR}/orascout.lock
log_file: ${TMPDIR}/orascout.log
insecure: true
repos:
  - ${REPO_NAME}:${REPO_TAG}
EOF

push_jar() {
  local jar_src="$1"
  local version="$2"
  pushd test/fixtures >/dev/null
  cp "${jar_src}" hello.jar
  oras push --plain-http \
    "${FULL_REF}" \
    --artifact-type application/vnd.dev.orascout.artifact.v1+json \
    --annotation "dev.orascout.v1.type=jar" \
    --annotation "dev.orascout.v1.source.file=hello.jar" \
    --annotation "dev.orascout.v1.target.path=${TARGET_FILE}" \
    --annotation "dev.orascout.v1.service.name=fake.service" \
    --annotation "dev.orascout.v1.service.manager=none" \
    --annotation "org.opencontainers.image.title=hello" \
    --annotation "org.opencontainers.image.version=${version}" \
    hello.jar:application/java-archive
  rm hello.jar
  popd >/dev/null
}

# --- 4. push v1 + cycle 1 -----------------------------------------------
echo "--- pushing v1 ---"
push_jar hello-v1.jar 1.0.0

echo "--- orascout cycle 1 (expect deploy) ---"
"${TMPDIR}/orascout" run -c "${CONFIG}" --once

[[ -f "${TARGET_FILE}" ]] || { echo "FAIL: target file not created"; exit 1; }
grep -q "version 1" "${TARGET_FILE}" || { echo "FAIL: v1 marker missing"; cat "${TARGET_FILE}"; exit 1; }
echo "OK: v1 deployed -> ${TARGET_FILE}"

STATE_DIGEST_V1=$(jq -r --arg k "${FULL_REF}" '.entries[$k].digest' "${TMPDIR}/state.json")
[[ -n "${STATE_DIGEST_V1}" && "${STATE_DIGEST_V1}" != "null" ]] || { echo "FAIL: state digest missing"; exit 1; }

# --- 5. push v2 + cycle 2 -----------------------------------------------
echo "--- pushing v2 ---"
push_jar hello-v2.jar 2.0.0

echo "--- orascout cycle 2 (expect deploy) ---"
"${TMPDIR}/orascout" run -c "${CONFIG}" --once

grep -q "version 2" "${TARGET_FILE}" || { echo "FAIL: v2 marker missing"; cat "${TARGET_FILE}"; exit 1; }
echo "OK: v2 deployed -> ${TARGET_FILE}"

STATE_DIGEST_V2=$(jq -r --arg k "${FULL_REF}" '.entries[$k].digest' "${TMPDIR}/state.json")
[[ "${STATE_DIGEST_V2}" != "${STATE_DIGEST_V1}" ]] || { echo "FAIL: state digest didn't change"; exit 1; }

# --- 6. cycle 3 (expect no-op) ------------------------------------------
echo "--- orascout cycle 3 (expect no-op) ---"
PREV_MTIME=$(stat -c '%Y' "${TARGET_FILE}" 2>/dev/null || stat -f '%m' "${TARGET_FILE}")
sleep 1
"${TMPDIR}/orascout" run -c "${CONFIG}" --once
NEW_MTIME=$(stat -c '%Y' "${TARGET_FILE}" 2>/dev/null || stat -f '%m' "${TARGET_FILE}")

[[ "${PREV_MTIME}" == "${NEW_MTIME}" ]] || {
  echo "FAIL: file was rewritten on no-op cycle (prev=${PREV_MTIME} new=${NEW_MTIME})"
  exit 1
}
echo "OK: no-op cycle did not rewrite target"
