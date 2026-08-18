#!/usr/bin/env bash
# Push a frontend dist/ directory as an orascout-deployable ORAS artifact.
#
# Usage:
#   ./push-static.sh <registry/repo:tag> <local-dist-dir> <web-root>
# Example:
#   ./push-static.sh docker.io/myorg/bar-ui:latest \
#                    ./dist \
#                    /var/www/html/Bar-UI
#
# The artifact will carry the entire dist/ tree as a single tar.gz layer; on
# pull, the puller unpacks it and the `static` strategy syncs into <web-root>.
set -euo pipefail

REF="${1:?usage: $0 REF DIST_DIR WEB_ROOT}"
DIST_DIR="${2:?usage: $0 REF DIST_DIR WEB_ROOT}"
WEB_ROOT="${3:?usage: $0 REF DIST_DIR WEB_ROOT}"

VERSION="${VERSION:-$(date -u +%Y%m%d%H%M%S)}"
DIST_BASE=$(basename "$DIST_DIR")

# Pack the dist tree into a tarball that lands at <pull-dir>/<DIST_BASE>/ on
# the target host, matching what the `static` strategy expects.
TMP_TAR="$(mktemp -d)/${DIST_BASE}.tar.gz"
tar -czf "$TMP_TAR" -C "$(dirname "$DIST_DIR")" "$DIST_BASE"

oras push "$REF" \
  --artifact-type application/vnd.dev.orascout.artifact.v1+json \
  --annotation "dev.orascout.v1.type=static" \
  --annotation "dev.orascout.v1.source.dir=${DIST_BASE}" \
  --annotation "dev.orascout.v1.target.path=${WEB_ROOT}" \
  --annotation "dev.orascout.v1.target.clear=true" \
  --annotation "dev.orascout.v1.target.mode=0755" \
  --annotation "org.opencontainers.image.title=${DIST_BASE}" \
  --annotation "org.opencontainers.image.version=${VERSION}" \
  --annotation "org.opencontainers.image.created=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  "${TMP_TAR}:application/vnd.dev.orascout.dist.v1.tar+gzip"

rm -rf "$(dirname "$TMP_TAR")"

cat <<EOF
NOTE: this script pushes the dist as a tarball. To use the `static` strategy
as-is, you'll either:
  1) ship the dist as plain files (multiple --files args to oras push), or
  2) keep the tarball but use type=hook-only with a small extract-and-rsync
     post-hook bundled into the artifact.
The pure-files path is simpler — see push-static-files.sh.
EOF
