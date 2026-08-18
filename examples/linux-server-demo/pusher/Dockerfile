# Pusher: builds real Java 8 / Tomcat 8.5 fixtures, then pushes them as
# ORAS artifacts with orascout-compatible annotations.

# ---------- stage 1: build JAR + WAR fixtures with real JDK 8 ------------
FROM eclipse-temurin:8-jdk-jammy AS fixtures

WORKDIR /work
# Build context is the orascout repo root (see docker-compose.yml). All COPY
# paths in this file are therefore prefixed with examples/linux-server-demo/.
COPY examples/linux-server-demo/pusher/src ./src
COPY examples/linux-server-demo/dist ./dist

# JAR v1 + v2: same HelloServer.java with VERSION_PLACEHOLDER swapped.
RUN for v in v1 v2; do \
      mkdir -p build/$v ; \
      sed "s/VERSION_PLACEHOLDER/$v/" src/HelloServer.java > build/$v/HelloServer.java ; \
      ( cd build/$v && javac HelloServer.java ) ; \
      ( cd build/$v && jar cfe hello-$v.jar HelloServer HelloServer.class ) ; \
    done

# WAR v1 + v2: ROOT.war = zip of WEB-INF/ + index.html, with the version
# string baked into the HTML.
RUN for v in v1 v2; do \
      mkdir -p build/war-$v/WEB-INF ; \
      cp src/war-template/WEB-INF/web.xml build/war-$v/WEB-INF/web.xml ; \
      sed "s/VERSION_PLACEHOLDER/$v/" src/war-template/index.html > build/war-$v/index.html ; \
      ( cd build/war-$v && jar -cf ../ROOT-$v.war . ) ; \
    done

# Collect everything the alpine stage needs into /out.
RUN mkdir -p /out \
 && cp build/v1/hello-v1.jar build/v2/hello-v2.jar /out/ \
 && cp build/ROOT-v1.war build/ROOT-v2.war /out/ \
 && cp -r dist/v1 /out/dist-v1 \
 && cp -r dist/v2 /out/dist-v2

# ---------- stage 2: lightweight pusher with oras CLI --------------------
FROM alpine:3.20
ARG ORAS_VERSION=1.2.0

RUN apk add --no-cache bash curl ca-certificates wget tar \
 && wget -q -O /tmp/oras.tgz \
      "https://github.com/oras-project/oras/releases/download/v${ORAS_VERSION}/oras_${ORAS_VERSION}_linux_amd64.tar.gz" \
 && tar -xzf /tmp/oras.tgz -C /usr/local/bin oras \
 && rm /tmp/oras.tgz \
 && oras version

# Drop the built fixtures and the push script into the image.
COPY --from=fixtures /out /fixtures
COPY examples/linux-server-demo/pusher/push-all.sh /push-all.sh
RUN chmod +x /push-all.sh

ENTRYPOINT ["bash", "/push-all.sh"]
CMD ["v1"]
