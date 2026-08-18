#!/usr/bin/env bash
# orascout install script.
#
# Downloads the latest pre-built binary for this host's arch, installs systemd
# units, and creates the config skeleton. Re-run is idempotent.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/arglampedakis/orascout/main/packaging/install.sh | sudo bash
#
# Environment overrides:
#   ORASCOUT_VERSION=v0.1.0      # pin a specific release
#   ORASCOUT_REPO=arglampedakis/orascout # for forks / private mirrors
set -euo pipefail

REPO="${ORASCOUT_REPO:-arglampedakis/orascout}"
VERSION="${ORASCOUT_VERSION:-latest}"
PREFIX="${PREFIX:-/usr/local}"
ETC="${ETC:-/etc/orascout}"
SYSTEMD_DIR="${SYSTEMD_DIR:-/etc/systemd/system}"

[[ $EUID -eq 0 ]] || { echo "install.sh must run as root (try sudo)"; exit 1; }

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *) echo "unsupported arch: $arch"; exit 1 ;;
esac

if [[ "$VERSION" == "latest" ]]; then
  url="https://github.com/${REPO}/releases/latest/download/orascout-linux-${GOARCH}"
else
  url="https://github.com/${REPO}/releases/download/${VERSION}/orascout-linux-${GOARCH}"
fi

echo "Downloading $url"
curl -fsSL "$url" -o "${PREFIX}/bin/orascout"
chmod +x "${PREFIX}/bin/orascout"

mkdir -p "${ETC}"
if [[ ! -f "${ETC}/config.yaml" ]]; then
  echo "Writing config skeleton to ${ETC}/config.yaml"
  cat >"${ETC}/config.yaml" <<'YAML'
# orascout config — see https://github.com/arglampedakis/orascout/blob/main/examples/config.yaml
registry_prefix: docker.io/myorg
poll_interval: 5m
repos:
  - hello:latest
YAML
fi

# Drop systemd units only if they don't already exist (so user edits aren't clobbered).
if [[ ! -f "${SYSTEMD_DIR}/orascout.service" ]]; then
  curl -fsSL "https://raw.githubusercontent.com/${REPO}/main/packaging/systemd/orascout.service" \
    -o "${SYSTEMD_DIR}/orascout.service"
fi
if [[ ! -f "${SYSTEMD_DIR}/orascout.timer" ]]; then
  curl -fsSL "https://raw.githubusercontent.com/${REPO}/main/packaging/systemd/orascout.timer" \
    -o "${SYSTEMD_DIR}/orascout.timer"
fi

systemctl daemon-reload
echo
echo "orascout installed."
echo "Next steps:"
echo "  1. Edit ${ETC}/config.yaml"
echo "  2. (Optional) put registry creds in ${ETC}/env  -- referenced as \$REGISTRY_USERNAME etc."
echo "  3. sudo systemctl enable --now orascout.timer"
echo "  4. journalctl -u orascout.service -f"
