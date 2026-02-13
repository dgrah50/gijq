#!/usr/bin/env bash
set -euo pipefail

REPO="dgrah50/gijq"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
VERSION="${1:-latest}"

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

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

if [ "$VERSION" = "latest" ]; then
  TAG="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
else
  TAG="$VERSION"
fi

if [ -z "$TAG" ]; then
  echo "failed to resolve release tag" >&2
  exit 1
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

ASSET="gijq_${TAG}_${OS}_${ARCH}.tar.gz"
CHECKSUM_ASSET="${ASSET}.sha256"
BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"

curl -fsSL "${BASE_URL}/${ASSET}" -o "${TMP_DIR}/${ASSET}"
curl -fsSL "${BASE_URL}/${CHECKSUM_ASSET}" -o "${TMP_DIR}/${CHECKSUM_ASSET}"

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$TMP_DIR" && sha256sum -c "$CHECKSUM_ASSET")
elif command -v shasum >/dev/null 2>&1; then
  EXPECTED="$(awk '{print $1}' "${TMP_DIR}/${CHECKSUM_ASSET}")"
  ACTUAL="$(shasum -a 256 "${TMP_DIR}/${ASSET}" | awk '{print $1}')"
  if [ "$EXPECTED" != "$ACTUAL" ]; then
    echo "checksum verification failed" >&2
    exit 1
  fi
else
  echo "warning: no sha256 tool found, skipping checksum verification" >&2
fi

tar -xzf "${TMP_DIR}/${ASSET}" -C "$TMP_DIR"
BIN="${TMP_DIR}/gijq_${TAG}_${OS}_${ARCH}"

if [ ! -f "$BIN" ]; then
  echo "expected binary not found in archive: $BIN" >&2
  exit 1
fi

if [ -w "$INSTALL_DIR" ]; then
  install -m 0755 "$BIN" "${INSTALL_DIR}/gijq"
else
  sudo install -m 0755 "$BIN" "${INSTALL_DIR}/gijq"
fi

echo "installed gijq ${TAG} to ${INSTALL_DIR}/gijq"
