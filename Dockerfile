# Multi-stage build for orascout.
#
# Stage 1 produces a static binary. Stage 2 packages it on a minimal base.
# The image is mainly useful for CI / local testing — production deployments
# should run the binary directly on the host so it can drive systemctl and
# write to host paths.

# --- build ---------------------------------------------------------------
FROM golang:1.22-alpine AS build

WORKDIR /src

# Cache deps separately from source.
COPY go.mod go.sum* ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/orascout \
    ./cmd/orascout

# --- runtime -------------------------------------------------------------
FROM alpine:3.20

# ca-certificates so we can talk to TLS registries (Docker Hub, GHCR, etc).
# tzdata so timestamps in logs are human-readable.
RUN apk add --no-cache ca-certificates tzdata

COPY --from=build /out/orascout /usr/local/bin/orascout

# Sensible defaults; user can override with -v mounts.
RUN mkdir -p /etc/orascout /var/lib/orascout /var/log/orascout

ENTRYPOINT ["/usr/local/bin/orascout"]
CMD ["run", "-c", "/etc/orascout/config.yaml"]
