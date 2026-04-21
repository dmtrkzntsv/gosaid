#!/usr/bin/env bash
# Build the GoSaid.app bundle for macOS.
#
# Inputs:
#   - An already-built gosaid binary at $DAEMON_BIN (defaults to out/gosaid).
#   - The Swift wrapper under macos/GosaidMenuBar.
#
# Output:
#   - $OUT_DIR/GoSaid.app (default: out/GoSaid.app)
#
# Env:
#   VERSION      Marketing version string (injected into Info.plist).
#   ARCH         arm64 | amd64 (mapped to arm64 | x86_64 for Swift).
#   DAEMON_BIN   Path to the gosaid Go binary to embed.  Default: out/gosaid.
#   OUT_DIR      Where to emit the .app.  Default: out.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${VERSION:-dev}"
ARCH="${ARCH:-$(uname -m)}"
case "$ARCH" in
    amd64) SWIFT_ARCH="x86_64" ;;
    arm64|aarch64) SWIFT_ARCH="arm64" ;;
    x86_64) SWIFT_ARCH="x86_64" ;;
    *) echo "build-macos-app: unsupported arch '$ARCH'" >&2; exit 1 ;;
esac
DAEMON_BIN="${DAEMON_BIN:-$ROOT/out/gosaid}"
OUT_DIR="${OUT_DIR:-$ROOT/out}"
APP="$OUT_DIR/GoSaid.app"
SWIFT_PKG="$ROOT/macos/GosaidMenuBar"

if [[ ! -x "$DAEMON_BIN" ]]; then
    echo "build-macos-app: daemon binary not found at $DAEMON_BIN" >&2
    echo "hint: run 'go build -o out/gosaid ./cmd/gosaid' first" >&2
    exit 1
fi

echo "build-macos-app: building Swift wrapper ($SWIFT_ARCH)"
pushd "$SWIFT_PKG" >/dev/null
swift build -c release --arch "$SWIFT_ARCH"
SWIFT_BIN=".build/$SWIFT_ARCH-apple-macosx/release/GosaidMenuBar"
if [[ ! -x "$SWIFT_BIN" ]]; then
    echo "build-macos-app: Swift output not found at $SWIFT_PKG/$SWIFT_BIN" >&2
    exit 1
fi
popd >/dev/null

echo "build-macos-app: assembling $APP"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

cp "$SWIFT_PKG/$SWIFT_BIN" "$APP/Contents/MacOS/GosaidMenuBar"
cp "$DAEMON_BIN" "$APP/Contents/MacOS/gosaid"
chmod +x "$APP/Contents/MacOS/GosaidMenuBar" "$APP/Contents/MacOS/gosaid"

# Inject the version into Info.plist.
sed "s/__VERSION__/$VERSION/g" \
    "$SWIFT_PKG/Resources/Info.plist" > "$APP/Contents/Info.plist"

# Ship entitlements alongside the app so sign-notarize.sh can find them.
cp "$SWIFT_PKG/Resources/entitlements.plist" "$APP/Contents/Resources/entitlements.plist"

echo "build-macos-app: wrote $APP"
