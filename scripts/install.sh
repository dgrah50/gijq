#!/usr/bin/env bash
set -euo pipefail

REPO="dgrah50/gijq"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
VERSION="${1:-latest}"

log() {
  echo "==> $*"
}

warn() {
  echo "warning: $*" >&2
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

log "Checking prerequisites"
need_cmd curl
need_cmd tar
need_cmd install

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
  linux|darwin) ;;
  *)
    echo "unsupported OS: $OS (supported: linux, darwin)" >&2
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)
    echo "unsupported architecture: $ARCH (supported: amd64, arm64)" >&2
    exit 1
    ;;
esac

log "Detected platform: ${OS}/${ARCH}"

if [ "$VERSION" = "latest" ]; then
  log "Resolving latest release version"
  TAG="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
else
  TAG="$VERSION"
fi

if [ -z "$TAG" ]; then
  echo "failed to resolve release tag" >&2
  exit 1
fi

log "Selected version: ${TAG}"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

ASSET="gijq_${TAG}_${OS}_${ARCH}.tar.gz"
CHECKSUM_ASSET="${ASSET}.sha256"
BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"

log "Downloading archive: ${ASSET}"
curl -fsSL "${BASE_URL}/${ASSET}" -o "${TMP_DIR}/${ASSET}"
log "Downloading checksum: ${CHECKSUM_ASSET}"
curl -fsSL "${BASE_URL}/${CHECKSUM_ASSET}" -o "${TMP_DIR}/${CHECKSUM_ASSET}"

log "Verifying checksum"
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$TMP_DIR" && sha256sum -c "$CHECKSUM_ASSET")
elif command -v shasum >/dev/null 2>&1; then
  EXPECTED="$(awk '{print $1}' "${TMP_DIR}/${CHECKSUM_ASSET}")"
  ACTUAL="$(shasum -a 256 "${TMP_DIR}/${ASSET}" | awk '{print $1}')"
  if [ "$EXPECTED" != "$ACTUAL" ]; then
    echo "checksum verification failed" >&2
    exit 1
  fi
  log "checksum OK"
else
  warn "no sha256 tool found, skipping checksum verification"
fi

log "Extracting archive"
tar -xzf "${TMP_DIR}/${ASSET}" -C "$TMP_DIR"
BIN="${TMP_DIR}/gijq_${TAG}_${OS}_${ARCH}"

if [ ! -f "$BIN" ]; then
  echo "expected binary not found in archive: $BIN" >&2
  exit 1
fi

log "Installing to ${INSTALL_DIR}/gijq"
if [ -w "$INSTALL_DIR" ]; then
  install -m 0755 "$BIN" "${INSTALL_DIR}/gijq"
else
  log "${INSTALL_DIR} is not writable by current user; requesting sudo"
  sudo install -m 0755 "$BIN" "${INSTALL_DIR}/gijq"
fi

log "Installed gijq ${TAG} to ${INSTALL_DIR}/gijq"
