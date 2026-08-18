#!/usr/bin/env bash
# Push the three demo artifacts (jar, war, static dist) to the registry.
#
# Usage (inside the pusher container):
#   /push-all.sh v1            (default)
#   /push-all.sh v2
set -euo pipefail

VERSION="${1:-v1}"
case "$VERSION" in v1|v2) ;; *) echo "usage: $0 [v1|v2]"; exit 2 ;; esac

REG="${REGISTRY_HOST:-registry:5000}"
NS="orascout-demo"
echo "=== pushing ${VERSION} to ${REG}/${NS}/* ==="

# JAR -> /opt/orascout/jars/hello.jar, restart hello-jar.service
WORK=$(mktemp -d)
cp "/fixtures/hello-${VERSION}.jar" "${WORK}/hello.jar"
( cd "$WORK" && oras push --plain-http \
    "${REG}/${NS}/hello-jar:latest" \
    --artifact-type application/vnd.dev.orascout.artifact.v1+json \
    --annotation "dev.orascout.v1.type=jar" \
    --annotation "dev.orascout.v1.source.file=hello.jar" \
    --annotation "dev.orascout.v1.target.path=/opt/orascout/jars/hello.jar" \
    --annotation "dev.orascout.v1.service.name=hello-jar.service" \
    --annotation "dev.orascout.v1.service.manager=systemd" \
    --annotation "dev.orascout.v1.target.owner=tomcat:tomcat" \
    --annotation "org.opencontainers.image.title=Hello JAR" \
    --annotation "org.opencontainers.image.version=${VERSION}" \
    hello.jar:application/java-archive )
rm -rf "$WORK"

# WAR -> /opt/tomcat/webapps/ROOT.war, clear webapps first, restart tomcat
WORK=$(mktemp -d)
cp "/fixtures/ROOT-${VERSION}.war" "${WORK}/ROOT.war"
( cd "$WORK" && oras push --plain-http \
    "${REG}/${NS}/hello-war:latest" \
    --artifact-type application/vnd.dev.orascout.artifact.v1+json \
    --annotation "dev.orascout.v1.type=war" \
    --annotation "dev.orascout.v1.source.file=ROOT.war" \
    --annotation "dev.orascout.v1.target.path=/opt/tomcat/webapps/ROOT.war" \
    --annotation "dev.orascout.v1.target.clear-parent=true" \
    --annotation "dev.orascout.v1.service.name=tomcat.service" \
    --annotation "dev.orascout.v1.service.manager=systemd" \
    --annotation "dev.orascout.v1.target.owner=tomcat:tomcat" \
    --annotation "org.opencontainers.image.title=Hello WAR" \
    --annotation "org.opencontainers.image.version=${VERSION}" \
    ROOT.war:application/java-archive )
rm -rf "$WORK"

# STATIC -> nginx /var/www/html, clear first
WORK=$(mktemp -d)
mkdir -p "${WORK}/dist"
cp -r "/fixtures/dist-${VERSION}/." "${WORK}/dist/"
( cd "$WORK" && oras push --plain-http --disable-path-validation \
    "${REG}/${NS}/hello-dist:latest" \
    --artifact-type application/vnd.dev.orascout.artifact.v1+json \
    --annotation "dev.orascout.v1.type=static" \
    --annotation "dev.orascout.v1.source.dir=dist" \
    --annotation "dev.orascout.v1.target.path=/var/www/html" \
    --annotation "dev.orascout.v1.target.clear=true" \
    --annotation "dev.orascout.v1.target.mode=0755" \
    --annotation "org.opencontainers.image.title=Hello Dist" \
    --annotation "org.opencontainers.image.version=${VERSION}" \
    dist/index.html:text/html )
rm -rf "$WORK"

cat <<EOF

=== ${VERSION} pushed. orascout's systemd timer fires every 30s. ===
Watch the deploys land:
  docker compose -f examples/linux-server-demo/docker-compose.yml exec linux-server \\
    journalctl -u orascout.service -f

Verify each:
  curl http://localhost:8082         # JAR HTTP server
  curl http://localhost:8081         # Tomcat-served WAR
  curl http://localhost:8080         # nginx-served static
EOF
