#!/usr/bin/env bash
# Build the Windows tray UI and package the distribution folder.
#
# Inputs:
#   - An already-built gosaid.exe at $DAEMON_EXE (defaults to out/gosaid.exe).
#   - The C# project under windows/GosaidUI.
#
# Output:
#   - $OUT_DIR/Gosaid/            directory containing GosaidUI.exe + gosaid.exe
#
# Env:
#   VERSION      Marketing version string (injected into the assembly).
#   DAEMON_EXE   Path to the gosaid.exe Go binary to embed.  Default: out/gosaid.exe.
#   OUT_DIR      Where to emit the Gosaid folder.  Default: out.
#   ARCH         win-x64 (default).  Accepted for forward compatibility.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${VERSION:-0.0.0}"
DAEMON_EXE="${DAEMON_EXE:-$ROOT/out/gosaid.exe}"
OUT_DIR="${OUT_DIR:-$ROOT/out}"
ARCH="${ARCH:-win-x64}"
CSPROJ="$ROOT/windows/GosaidUI/GosaidUI.csproj"
PUBLISH_DIR="$ROOT/windows/GosaidUI/bin/publish"

if [[ ! -f "$DAEMON_EXE" ]]; then
    echo "build-windows-ui: daemon binary not found at $DAEMON_EXE" >&2
    echo "hint: run 'GOOS=windows GOARCH=amd64 go build -o out/gosaid.exe ./cmd/gosaid' first" >&2
    exit 1
fi

if ! command -v dotnet >/dev/null; then
    echo "build-windows-ui: dotnet SDK (>= 9.0) is required" >&2
    exit 1
fi

# Strip any prerelease/build suffix for the numeric assembly version —
# .NET requires A.B.C.D with numeric components only.  We keep the full
# VERSION in InformationalVersion for humans.
numeric_version="$(echo "$VERSION" | sed -E 's/^([0-9]+(\.[0-9]+){0,3}).*/\1/')"
case "$numeric_version" in
    ''|*[!0-9.]*) numeric_version="0.0.0" ;;
esac
# Pad to A.B.C.D so AssemblyVersion parses cleanly.
while [[ "$(echo "$numeric_version" | awk -F. '{print NF}')" -lt 4 ]]; do
    numeric_version="$numeric_version.0"
done

echo "build-windows-ui: dotnet publish $ARCH (version=$VERSION assembly=$numeric_version)"
rm -rf "$PUBLISH_DIR"
dotnet publish "$CSPROJ" \
    -c Release \
    -r "$ARCH" \
    -o "$PUBLISH_DIR" \
    -p:AssemblyVersion="$numeric_version" \
    -p:FileVersion="$numeric_version" \
    -p:InformationalVersion="$VERSION" \
    -p:Version="$numeric_version"

UI_EXE="$PUBLISH_DIR/GosaidUI.exe"
if [[ ! -f "$UI_EXE" ]]; then
    echo "build-windows-ui: dotnet publish did not produce $UI_EXE" >&2
    exit 1
fi

DIST="$OUT_DIR/Gosaid"
echo "build-windows-ui: assembling $DIST"
rm -rf "$DIST"
mkdir -p "$DIST"
cp "$UI_EXE" "$DIST/GosaidUI.exe"
cp "$DAEMON_EXE" "$DIST/gosaid.exe"

# Copy debug/pdb artefacts too so crash dumps are actionable without a build server.
for f in "$PUBLISH_DIR"/*.pdb; do
    [[ -e "$f" ]] && cp "$f" "$DIST/"
done

echo "build-windows-ui: wrote $DIST"
