#!/usr/bin/env bash
# Sign and notarize a macOS Mach-O binary in place.
# Usage: sign-notarize.sh <path-to-binary>
# No-ops on non-darwin builds so it can be wired into goreleaser hooks unconditionally.
#
# Required env vars (when running on a darwin binary):
#   MACOS_DEVELOPER_ID_APPLICATION  e.g. "Developer ID Application: Name (TEAMID)"
#   NOTARY_APPLE_ID                 Apple ID email
#   NOTARY_TEAM_ID                  10-char Team ID
#   NOTARY_PASSWORD                 app-specific password from appleid.apple.com

set -euo pipefail

BIN="${1:?usage: $0 <binary>}"

case "$BIN" in
    *darwin*) ;;
    *) exit 0 ;;
esac

if [[ ! -f "$BIN" ]]; then
    echo "sign-notarize: binary not found: $BIN" >&2
    exit 1
fi

: "${MACOS_DEVELOPER_ID_APPLICATION:?required}"
: "${NOTARY_APPLE_ID:?required}"
: "${NOTARY_TEAM_ID:?required}"
: "${NOTARY_PASSWORD:?required}"

ENTITLEMENTS="$(cd "$(dirname "$0")" && pwd)/entitlements.plist"

echo "sign-notarize: codesign $BIN"
codesign --sign "$MACOS_DEVELOPER_ID_APPLICATION" \
    --options runtime \
    --timestamp \
    --entitlements "$ENTITLEMENTS" \
    --force \
    "$BIN"

codesign --verify --verbose=2 "$BIN"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
ZIP="$WORK/notarize.zip"

ditto -c -k --keepParent "$BIN" "$ZIP"

echo "sign-notarize: submitting to Apple notary"
xcrun notarytool submit "$ZIP" \
    --apple-id "$NOTARY_APPLE_ID" \
    --team-id "$NOTARY_TEAM_ID" \
    --password "$NOTARY_PASSWORD" \
    --wait

echo "sign-notarize: done"
