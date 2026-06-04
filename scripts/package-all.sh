#!/usr/bin/env bash
# Build one VSIX per supported platform, each bundling ONLY that
# platform's sidecar + harness binaries.
set -euo pipefail

REPO_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

ALL=(darwin-arm64 darwin-amd64 linux-amd64 linux-arm64 windows-amd64)

vsce_target_for() {
  case "$1" in
    darwin-arm64)   echo darwin-arm64 ;;
    darwin-amd64)   echo darwin-x64 ;;
    linux-amd64)    echo linux-x64 ;;
    linux-arm64)    echo linux-arm64 ;;
    windows-amd64)  echo win32-x64 ;;
    *) echo "" ;;
  esac
}

if [[ $# -gt 0 ]]; then
  TARGETS=("$@")
else
  TARGETS=("${ALL[@]}")
fi

make sidecars-all build-ext

STASH="$(mktemp -d)"
cleanup() {
  rm -rf extension/bin/* 2>/dev/null || true
  if compgen -G "$STASH/*" > /dev/null; then
    mv "$STASH"/* extension/bin/ 2>/dev/null || true
  fi
  rmdir "$STASH" 2>/dev/null || true
}
trap cleanup EXIT
mv extension/bin/* "$STASH"/

for p in "${TARGETS[@]}"; do
  target="$(vsce_target_for "$p")"
  if [[ -z "$target" ]]; then
    echo "unknown platform: $p" >&2
    exit 1
  fi
  src="$STASH/$p"
  if [[ ! -d "$src" ]]; then
    echo "no binaries built for $p (expected $src)" >&2
    exit 1
  fi
  echo "==> packaging $p (vsce target=$target)"
  rm -rf extension/bin/*
  cp -R "$src" "extension/bin/$p"
  (
    cd extension
    npx --yes @vscode/vsce package \
      --no-dependencies \
      --target "$target" \
      --out "foundry-copilot-${target}.vsix"
  )
done

echo "==> done. VSIXes in extension/*.vsix"
ls -lh extension/*.vsix
