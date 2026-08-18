#!/usr/bin/env bash
# Push a WAR as an orascout-deployable ORAS artifact.
#
# Usage:
#   ./push-war.sh <registry/repo:tag> <local.war> <target-path> <service.unit>
# Example:
#   ./push-war.sh docker.io/myorg/bar:latest \
#                 ./ROOT.war \
#                 /opt/tomcat/instances/Bar/webapps/ROOT.war \
#                 bar.service
set -euo pipefail

REF="${1:?usage: $0 REF WAR TARGET_PATH SERVICE_UNIT}"
WAR="${2:?usage: $0 REF WAR TARGET_PATH SERVICE_UNIT}"
TARGET_PATH="${3:?usage: $0 REF WAR TARGET_PATH SERVICE_UNIT}"
SERVICE_UNIT="${4:?usage: $0 REF WAR TARGET_PATH SERVICE_UNIT}"

VERSION="${VERSION:-$(date -u +%Y%m%d%H%M%S)}"
WAR_BASENAME=$(basename "$WAR")

oras push "$REF" \
  --artifact-type application/vnd.dev.orascout.artifact.v1+json \
  --annotation "dev.orascout.v1.type=war" \
  --annotation "dev.orascout.v1.source.file=${WAR_BASENAME}" \
  --annotation "dev.orascout.v1.target.path=${TARGET_PATH}" \
  --annotation "dev.orascout.v1.service.name=${SERVICE_UNIT}" \
  --annotation "dev.orascout.v1.service.manager=systemd-user" \
  --annotation "dev.orascout.v1.target.clear-parent=true" \
  --annotation "org.opencontainers.image.title=${WAR_BASENAME}" \
  --annotation "org.opencontainers.image.version=${VERSION}" \
  --annotation "org.opencontainers.image.created=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  "${WAR}:application/java-archive"
