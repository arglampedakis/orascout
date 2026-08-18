# Sandbox "Linux server" mirroring a typical production stack.
#
# Pre-installed BEFORE orascout (so services can start with placeholder content
# while you install orascout by hand):
#   - OpenJDK 8
#   - Apache Tomcat 8.5.100 at /opt/tomcat (CATALINA_HOME)
#   - A Bar instance at /opt/tomcat/instances/Bar (CATALINA_BASE)
#   - tomcat user with home /home/tomcat
#   - /home/tomcat/jar-services/foo.jar             (placeholder, real running JAR)
#   - /opt/tomcat/instances/Bar/webapps/ROOT.war    (placeholder)
#   - /var/www/html/Bar-UI/index.html               (placeholder)
#   - nginx, foo.service, bar.service — all enabled at boot
#
# STAGED for manual install (NOT auto-deployed):
#   - /staging/systemd/orascout.{service,timer}
#   - /staging/config.yaml.example
#   - /staging/env.example
#
# The orascout binary itself is NOT baked in — INSTALL-STEPS.md has the
# user `curl` it from the GitHub release, matching the real-host workflow.
# This stage layout lets the user do `cp /staging/...` and one `curl ...`
# as part of the walkthrough — same actions they'd do on a real host.

# ============================================================
# Stage 1: compile placeholders (real foo.jar + real ROOT.war)
# ============================================================
FROM eclipse-temurin:8-jdk-jammy AS placeholders

WORKDIR /work
COPY examples/sandbox-server/foo-placeholder/HelloFoo.java .
RUN javac HelloFoo.java \
 && jar cfe foo.jar HelloFoo HelloFoo.class

COPY examples/sandbox-server/war-placeholder/ war/
RUN cd war && jar -cf /work/ROOT.war .

# ============================================================
# Stage 2: the systemd sandbox
# ============================================================
# Note: orascout itself is NOT baked into this image. INSTALL-STEPS.md has
# the user `curl` the binary from the GitHub release, exactly as they would
# on a real Ubuntu host. /staging/ only holds the systemd units and config
# templates — things a sysadmin would realistically have on hand when
# setting up the daemon.
FROM jrei/systemd-ubuntu:22.04

ARG TOMCAT_VERSION=8.5.100
ENV DEBIAN_FRONTEND=noninteractive \
    JAVA_HOME=/usr/lib/jvm/java-8-openjdk-amd64

# Base packages. nano + jq + sudo are conveniences for the interactive install
# walkthrough. openjdk-8 lives in universe on Ubuntu 22.04.
RUN apt-get update \
 && apt-get install -y --no-install-recommends software-properties-common \
 && add-apt-repository -y universe \
 && apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates curl wget nano jq sudo less \
      openjdk-8-jre-headless \
      nginx \
 && rm -rf /var/lib/apt/lists/*

# tomcat user with home /home/tomcat — common convention for stacks that
# treat /home/tomcat as the service-owner home alongside Tomcat at /opt/tomcat.
RUN groupadd --system tomcat \
 && useradd --system --gid tomcat --create-home --home-dir /home/tomcat --shell /bin/bash tomcat

# Apache Tomcat 8.5 — install at /opt/tomcat (CATALINA_HOME).
RUN curl -fsSL "https://archive.apache.org/dist/tomcat/tomcat-8/v${TOMCAT_VERSION}/bin/apache-tomcat-${TOMCAT_VERSION}.tar.gz" \
      -o /tmp/tomcat.tgz \
 && mkdir -p /opt/tomcat \
 && tar -xzf /tmp/tomcat.tgz -C /opt/tomcat --strip-components=1 \
 && rm /tmp/tomcat.tgz \
 && rm -rf /opt/tomcat/webapps/* \
 && chmod +x /opt/tomcat/bin/*.sh

# Bar Tomcat instance (CATALINA_BASE) at /opt/tomcat/instances/Bar.
# This follows the convention WARS_DIR=/opt/tomcat/instances + per-app
# subdirectory — one Tomcat install hosting multiple isolated app instances.
RUN mkdir -p /opt/tomcat/instances/Bar/conf \
             /opt/tomcat/instances/Bar/logs \
             /opt/tomcat/instances/Bar/temp \
             /opt/tomcat/instances/Bar/webapps \
             /opt/tomcat/instances/Bar/work \
 && cp /opt/tomcat/conf/server.xml \
       /opt/tomcat/conf/web.xml \
       /opt/tomcat/conf/context.xml \
       /opt/tomcat/conf/tomcat-users.xml \
       /opt/tomcat/conf/catalina.policy \
       /opt/tomcat/conf/catalina.properties \
       /opt/tomcat/conf/logging.properties \
       /opt/tomcat/instances/Bar/conf/ \
 && chown -R tomcat:tomcat /opt/tomcat

# JAR services directory (JARS_DIR=/home/tomcat/jar-services).
RUN mkdir -p /home/tomcat/jar-services \
 && chown -R tomcat:tomcat /home/tomcat/jar-services

# Frontend directory (DIST_DIR=/var/www/html, app subdir Bar-UI).
RUN mkdir -p /var/www/html/Bar-UI

# Placeholder artifacts — copied from the placeholders/ build stage.
COPY --from=placeholders /work/foo.jar  /home/tomcat/jar-services/foo.jar
COPY --from=placeholders /work/ROOT.war /opt/tomcat/instances/Bar/webapps/ROOT.war
COPY examples/sandbox-server/placeholders/index.html /var/www/html/Bar-UI/index.html
RUN chown tomcat:tomcat /home/tomcat/jar-services/foo.jar \
                        /opt/tomcat/instances/Bar/webapps/ROOT.war

# nginx config — serves /var/www/html/Bar-UI at http://<host>/Bar-UI/
COPY examples/sandbox-server/nginx-default.conf /etc/nginx/sites-available/default

# systemd units for the services (NOT orascout — staged separately).
COPY examples/sandbox-server/systemd/foo.service /etc/systemd/system/foo.service
COPY examples/sandbox-server/systemd/bar.service /etc/systemd/system/bar.service

# Enable services at boot. orascout.timer is NOT enabled — that's a step in
# INSTALL-STEPS.md you do yourself.
RUN systemctl enable foo.service \
 && systemctl enable bar.service \
 && systemctl enable nginx.service

# orascout staging area: systemd units + config templates. The binary itself
# is downloaded by the user in Step 4a of INSTALL-STEPS.md (curl from the
# GitHub release).
COPY examples/sandbox-server/staging/orascout.service     /staging/systemd/orascout.service
COPY examples/sandbox-server/staging/orascout.timer       /staging/systemd/orascout.timer
COPY examples/sandbox-server/staging/config.yaml.example  /staging/config.yaml.example
COPY examples/sandbox-server/staging/env.example          /staging/env.example

# Healthcheck — gives docker compose --wait something to wait on.
HEALTHCHECK --interval=5s --timeout=3s --start-period=45s --retries=30 \
  CMD systemctl is-system-running --quiet \
   || systemctl is-system-running | grep -q "degraded\|running"
