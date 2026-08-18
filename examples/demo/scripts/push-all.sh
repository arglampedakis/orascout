#!/usr/bin/env bash
# Push all three demo artifacts (jar, war, static dist) to the registry.
#
# Usage:
#   ./push-all.sh [v1|v2]              (default v1)
#
# Talks to ${REGISTRY_HOST:-registry:5000}. From inside the docker-compose
# network that's "registry:5000"; from your host machine it's "localhost:5000".
#
# Example, from your host shell (oras CLI installed):
#   REGISTRY_HOST=localhost:5000 ./examples/demo/scripts/push-all.sh v2

set -euo pipefail

VERSION="${1:-v1}"
case "$VERSION" in
  v1|v2) ;;
  *) echo "usage: $0 [v1|v2]"; exit 2 ;;
esac

REGISTRY_HOST="${REGISTRY_HOST:-registry:5000}"
NS="orascout-demo"
FIXTURES="$(cd "$(dirname "$0")/.." && pwd)/fixtures"

echo "=== pushing ${VERSION} to ${REGISTRY_HOST}/${NS}/* ==="

# -------------------------------------------------------------------------
# 1. JAR  -> orascout will copy to /srv/jars/hello.jar
# -------------------------------------------------------------------------
TMP=$(mktemp -d)
cp "${FIXTURES}/hello-${VERSION}.jar" "${TMP}/hello.jar"
( cd "${TMP}" && \
  oras push --plain-http \
    "${REGISTRY_HOST}/${NS}/hello-jar:latest" \
    --artifact-type application/vnd.dev.orascout.artifact.v1+json \
    --annotation "dev.orascout.v1.type=jar" \
    --annotation "dev.orascout.v1.source.file=hello.jar" \
    --annotation "dev.orascout.v1.target.path=/srv/jars/hello.jar" \
    --annotation "dev.orascout.v1.service.name=demo-jar.service" \
    --annotation "dev.orascout.v1.service.manager=none" \
    --annotation "org.opencontainers.image.title=Hello JAR" \
    --annotation "org.opencontainers.image.version=${VERSION}" \
    hello.jar:application/java-archive )
rm -rf "$TMP"

# -------------------------------------------------------------------------
# 2. WAR  -> orascout will clear /srv/webapps and copy ROOT.war into it
# -------------------------------------------------------------------------
TMP=$(mktemp -d)
cp "${FIXTURES}/ROOT-${VERSION}.war" "${TMP}/ROOT.war"
( cd "${TMP}" && \
  oras push --plain-http \
    "${REGISTRY_HOST}/${NS}/hello-war:latest" \
    --artifact-type application/vnd.dev.orascout.artifact.v1+json \
    --annotation "dev.orascout.v1.type=war" \
    --annotation "dev.orascout.v1.source.file=ROOT.war" \
    --annotation "dev.orascout.v1.target.path=/srv/webapps/ROOT.war" \
    --annotation "dev.orascout.v1.target.clear-parent=true" \
    --annotation "dev.orascout.v1.service.name=demo-war.service" \
    --annotation "dev.orascout.v1.service.manager=none" \
    --annotation "org.opencontainers.image.title=Hello WAR" \
    --annotation "org.opencontainers.image.version=${VERSION}" \
    ROOT.war:application/java-archive )
rm -rf "$TMP"

# -------------------------------------------------------------------------
# 3. STATIC dist  -> orascout will sync into /srv/dist (nginx serves from there)
# -------------------------------------------------------------------------
# We push the dist as individual files under a dist/ prefix so the artifact
# pulls into <artifactDir>/dist/* and the static strategy's source.dir=dist
# picks it up.
TMP=$(mktemp -d)
mkdir -p "${TMP}/dist"
cp -r "${FIXTURES}/dist-${VERSION}/." "${TMP}/dist/"
( cd "${TMP}" && \
  oras push --plain-http --disable-path-validation \
    "${REGISTRY_HOST}/${NS}/hello-dist:latest" \
    --artifact-type application/vnd.dev.orascout.artifact.v1+json \
    --annotation "dev.orascout.v1.type=static" \
    --annotation "dev.orascout.v1.source.dir=dist" \
    --annotation "dev.orascout.v1.target.path=/srv/dist" \
    --annotation "dev.orascout.v1.target.clear=true" \
    --annotation "org.opencontainers.image.title=Hello Dist" \
    --annotation "org.opencontainers.image.version=${VERSION}" \
    dist/index.html:text/html )
rm -rf "$TMP"

echo
echo "=== ${VERSION} pushed. orascout will pick it up within its poll interval. ==="
echo "    orascout daemon logs:   docker compose -f examples/demo/docker-compose.yml logs -f orascout"
echo "    inspect jar target:     docker compose -f examples/demo/docker-compose.yml exec orascout ls -la /srv/jars"
echo "    inspect war target:     docker compose -f examples/demo/docker-compose.yml exec orascout ls -la /srv/webapps"
echo "    static page:            open http://localhost:8080"
