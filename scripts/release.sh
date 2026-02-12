#!/usr/bin/env bash
set -euo pipefail

BUMP_TYPE="${1:-patch}"

case "$BUMP_TYPE" in
  major|minor|patch) ;;
  *)
    echo "usage: scripts/release.sh [major|minor|patch]" >&2
    exit 1
    ;;
esac

if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "working tree is not clean; commit or stash changes first" >&2
  exit 1
fi

if ! git rev-parse --git-dir >/dev/null 2>&1; then
  echo "not in a git repository" >&2
  exit 1
fi

LATEST_TAG="$({ git tag --list 'v*' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' || true; } | sort -V | tail -n 1)"

if [ -z "$LATEST_TAG" ]; then
  LATEST_TAG="v0.0.0"
fi

VERSION_CORE="${LATEST_TAG#v}"
IFS='.' read -r MAJOR MINOR PATCH <<< "$VERSION_CORE"

case "$BUMP_TYPE" in
  major)
    MAJOR=$((MAJOR + 1))
    MINOR=0
    PATCH=0
    ;;
  minor)
    MINOR=$((MINOR + 1))
    PATCH=0
    ;;
  patch)
    PATCH=$((PATCH + 1))
    ;;
esac

NEW_TAG="v${MAJOR}.${MINOR}.${PATCH}"

if git rev-parse "$NEW_TAG" >/dev/null 2>&1; then
  echo "tag already exists: $NEW_TAG" >&2
  exit 1
fi

echo "latest: $LATEST_TAG"
echo "next:   $NEW_TAG"

git tag -a "$NEW_TAG" -m "Release $NEW_TAG"
git push origin "$NEW_TAG"

echo "created and pushed $NEW_TAG"
