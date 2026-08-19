# A "fake Linux server" container for testing orascout's real-systemd path
# (TUTORIAL.md Part B) without a VM.
#
# Includes:
#   - systemd as PID 1 (via jrei/systemd-ubuntu:22.04 base)
#   - OpenJDK 8 (matches the legacy stack orascout was designed for)
#   - Apache Tomcat 8.5.100 (from Apache archive)
#   - nginx (serves the static /var/www/html target)
#   - orascout binary + systemd timer (runs every 30s for fast demo feedback)
#
# REQUIRES `privileged: true` in docker-compose because systemd inside an
# unprivileged container can't manage cgroups. Never do this in production —
# this image is a development-time integration-test rig only.

# ---------- stage 1: build orascout from the repo source ------------------
FROM golang:1.25-alpine AS orascout-build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o /out/orascout ./cmd/orascout

# ---------- stage 2: the systemd "host" -----------------------------------
FROM jrei/systemd-ubuntu:22.04

ARG TOMCAT_VERSION=8.5.100
ENV DEBIAN_FRONTEND=noninteractive \
    JAVA_HOME=/usr/lib/jvm/java-8-openjdk-amd64 \
    CATALINA_HOME=/opt/tomcat \
    CATALINA_BASE=/opt/tomcat

# Base packages. openjdk-8-jre-headless is in universe on 22.04; enable it
# explicitly to be safe across mirrors.
RUN apt-get update \
 && apt-get install -y --no-install-recommends software-properties-common \
 && add-apt-repository -y universe \
 && apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates curl wget \
      openjdk-8-jre-headless \
      nginx \
      jq \
 && rm -rf /var/lib/apt/lists/*

# Tomcat 8.5 from Apache archive (permanent URL).
RUN groupadd --system tomcat \
 && useradd --system --gid tomcat --home-dir /opt/tomcat --shell /bin/false tomcat \
 && curl -fsSL "https://archive.apache.org/dist/tomcat/tomcat-8/v${TOMCAT_VERSION}/bin/apache-tomcat-${TOMCAT_VERSION}.tar.gz" \
      -o /tmp/tomcat.tgz \
 && mkdir -p /opt/tomcat \
 && tar -xzf /tmp/tomcat.tgz -C /opt/tomcat --strip-components=1 \
 && rm /tmp/tomcat.tgz \
 && rm -rf /opt/tomcat/webapps/* \
 && chown -R tomcat:tomcat /opt/tomcat \
 && chmod +x /opt/tomcat/bin/*.sh

# Target directories for the three deploys. Owned by tomcat so the services
# can read them; orascout (running as root via systemd) writes here.
RUN mkdir -p /opt/orascout/jars /var/www/html /etc/orascout /var/lib/orascout /var/log/orascout \
 && chown tomcat:tomcat /opt/orascout/jars

# orascout binary.
COPY --from=orascout-build /out/orascout /usr/local/bin/orascout

# Config + systemd units.
COPY examples/linux-server-demo/linux-server/orascout-config.yaml /etc/orascout/config.yaml
COPY examples/linux-server-demo/linux-server/systemd/tomcat.service       /etc/systemd/system/tomcat.service
COPY examples/linux-server-demo/linux-server/systemd/hello-jar.service    /etc/systemd/system/hello-jar.service
COPY examples/linux-server-demo/linux-server/systemd/orascout.service     /etc/systemd/system/orascout.service
COPY examples/linux-server-demo/linux-server/systemd/orascout.timer       /etc/systemd/system/orascout.timer
COPY examples/linux-server-demo/linux-server/nginx-default.conf           /etc/nginx/sites-available/default

# Enable units so they start when systemd boots in the container.
# hello-jar.service is intentionally NOT enabled at boot — its JAR doesn't
# exist yet. The first orascout cycle creates it, then the service can start.
RUN systemctl enable tomcat.service \
 && systemctl enable nginx.service \
 && systemctl enable orascout.timer

# Health hint for compose. systemd is PID 1 (inherited from the base image).
HEALTHCHECK --interval=5s --timeout=3s --start-period=20s --retries=20 \
  CMD systemctl is-system-running --quiet || systemctl is-system-running | grep -q "degraded\|running"
