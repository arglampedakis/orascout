#!/usr/bin/env bash
# Push a one-shot artifact (e.g. a DB migration JAR) — the puller will run it
# to completion every time the digest changes. No service lifecycle.
#
# Usage:
#   ./push-runonce.sh <registry/repo:tag> <local.jar> "<command-template>"
# Example:
#   ./push-runonce.sh docker.io/myorg/db-migration:latest \
#                     ./db-migration.jar \
#                     "java -jar -Dfile.encoding=UTF-8 {file}"
#
# {file} in the command template is substituted at deploy time with the
# absolute path to the pulled source file.
set -euo pipefail

REF="${1:?usage: $0 REF JAR \"CMD_TEMPLATE\"}"
JAR="${2:?usage: $0 REF JAR \"CMD_TEMPLATE\"}"
CMD="${3:?usage: $0 REF JAR \"CMD_TEMPLATE\"}"

VERSION="${VERSION:-$(date -u +%Y%m%d%H%M%S)}"
JAR_BASENAME=$(basename "$JAR")

oras push "$REF" \
  --artifact-type application/vnd.dev.orascout.artifact.v1+json \
  --annotation "dev.orascout.v1.type=run-once" \
  --annotation "dev.orascout.v1.source.file=${JAR_BASENAME}" \
  --annotation "dev.orascout.v1.runonce.command=${CMD}" \
  --annotation "org.opencontainers.image.title=${JAR_BASENAME}" \
  --annotation "org.opencontainers.image.version=${VERSION}" \
  --annotation "org.opencontainers.image.created=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  "${JAR}:application/java-archive"
