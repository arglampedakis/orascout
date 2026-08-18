#!/usr/bin/env bash
# Push a JAR as an orascout-deployable ORAS artifact.
#
# Usage:
#   ./push-jar.sh <registry/repo:tag> <local.jar> <target-path> <service.unit>
# Example:
#   ./push-jar.sh docker.io/myorg/foo:latest \
#                 ./foo.jar \
#                 /home/tomcat/jar-services/foo.jar \
#                 foo.service
set -euo pipefail

REF="${1:?usage: $0 REF JAR TARGET_PATH SERVICE_UNIT}"
JAR="${2:?usage: $0 REF JAR TARGET_PATH SERVICE_UNIT}"
TARGET_PATH="${3:?usage: $0 REF JAR TARGET_PATH SERVICE_UNIT}"
SERVICE_UNIT="${4:?usage: $0 REF JAR TARGET_PATH SERVICE_UNIT}"

VERSION="${VERSION:-$(date -u +%Y%m%d%H%M%S)}"
JAR_BASENAME=$(basename "$JAR")

oras push "$REF" \
  --artifact-type application/vnd.dev.orascout.artifact.v1+json \
  --annotation "dev.orascout.v1.type=jar" \
  --annotation "dev.orascout.v1.source.file=${JAR_BASENAME}" \
  --annotation "dev.orascout.v1.target.path=${TARGET_PATH}" \
  --annotation "dev.orascout.v1.service.name=${SERVICE_UNIT}" \
  --annotation "dev.orascout.v1.service.manager=systemd-user" \
  --annotation "org.opencontainers.image.title=${JAR_BASENAME}" \
  --annotation "org.opencontainers.image.version=${VERSION}" \
  --annotation "org.opencontainers.image.created=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  "${JAR}:application/java-archive"
