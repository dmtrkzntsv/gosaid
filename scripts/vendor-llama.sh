#!/usr/bin/env bash
# Vendors llama.cpp core library sources into internal/llama/cvendor/.
# Only src/ and include/llama.h — no common/, server, or multimodal.
# Go shim files (*.go) are preserved. Fails if the tag's ggml version does
# not match the vendored internal/ggml tree (re-run vendor-ggml.sh first).
# Usage: scripts/vendor-llama.sh [tag]
set -euo pipefail

cd "$(dirname "$0")/.."
source scripts/vendor-versions.env
TAG="${1:-$LLAMA_TAG}"
DEST="internal/llama/cvendor"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

git clone --depth 1 --branch "$TAG" https://github.com/ggml-org/llama.cpp "$TMP/l"

# ggml compatibility gate.
VENDORED="$(sed -n 's/#define GGML_VERSION "\(.*\)"/\1/p' internal/ggml/cvendor/src/ggml-version-defs.h)"
UPSTREAM="$(sed -n \
  -e 's/.*set(GGML_VERSION_MAJOR \([0-9]*\)).*/\1/p' \
  -e 's/.*set(GGML_VERSION_MINOR \([0-9]*\)).*/\1/p' \
  -e 's/.*set(GGML_VERSION_PATCH \([0-9]*\)).*/\1/p' \
  "$TMP/l/ggml/CMakeLists.txt" | paste -sd. -)"
if [ "$VENDORED" != "$UPSTREAM" ]; then
  echo "error: vendored ggml is $VENDORED but llama.cpp $TAG ships ggml $UPSTREAM" >&2
  echo "       run scripts/vendor-ggml.sh $TAG first (and re-verify whisper builds)" >&2
  exit 1
fi

if [ -d "$DEST" ]; then
  find "$DEST" -type f ! -name '*.go' -delete
  find "$DEST" -type d -empty -delete
fi
mkdir -p "$DEST/include" "$DEST/src"

cp "$TMP/l/include/llama.h" "$DEST/include/"
find "$TMP/l/src" -maxdepth 1 -type f \( -name '*.cpp' -o -name '*.h' \) \
  -exec cp {} "$DEST/src/" \;

# Each src/ subdirectory needs its own cgo shim package (cgo compiles only
# the sources in a package's own directory). Copy them and list what was
# found so a new upstream subdirectory is noticed, not silently dropped.
for d in "$TMP/l/src"/*/; do
  name="$(basename "$d")"
  cp -r "$d" "$DEST/src/$name"
  if [ ! -f "$DEST/src/$name/build.go" ]; then
    echo "warning: $DEST/src/$name has no build.go shim — add one or the sources will not compile" >&2
  fi
done

find "$DEST" -name 'CMakeLists.txt' -delete
find "$DEST" -name '*.cmake' -delete
find "$DEST" -type d -empty -delete

echo "$TAG" > "$DEST/VERSION"
echo "vendored llama.cpp $TAG into $DEST"
