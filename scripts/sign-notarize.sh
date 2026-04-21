#!/usr/bin/env bash
# Sign and notarize a macOS artefact.
#
# Accepts either:
#   - a Mach-O binary (legacy, e.g. "out/gosaid")
#   - a .app bundle   (new, e.g. "out/GoSaid.app")
#
# For bundles, every Mach-O inside Contents/MacOS/ is signed individually
# with the runtime hardening + entitlements, the outer bundle is signed
# last (required for --deep to produce a valid signature), and the whole
# bundle is submitted as a zip to Apple notary.
#
# Required env vars:
#   MACOS_DEVELOPER_ID_APPLICATION  e.g. "Developer ID Application: Name (TEAMID)"
#   NOTARY_APPLE_ID                 Apple ID email
#   NOTARY_TEAM_ID                  10-char Team ID
#   NOTARY_PASSWORD                 app-specific password from appleid.apple.com

set -euo pipefail

TARGET="${1:?usage: $0 <binary-or-app>}"

# No-op when invoked against non-darwin artefacts so CI can wire it in
# unconditionally per matrix target.
case "$TARGET" in
    *.app|*darwin*) ;;
    *) exit 0 ;;
esac

if [[ ! -e "$TARGET" ]]; then
    echo "sign-notarize: target not found: $TARGET" >&2
    exit 1
fi

: "${MACOS_DEVELOPER_ID_APPLICATION:?required}"
: "${NOTARY_APPLE_ID:?required}"
: "${NOTARY_TEAM_ID:?required}"
: "${NOTARY_PASSWORD:?required}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ENTITLEMENTS_DEFAULT="$SCRIPT_DIR/entitlements.plist"

sign_file() {
    local path="$1"
    local entitlements="$2"
    echo "sign-notarize: codesign $path"
    codesign --sign "$MACOS_DEVELOPER_ID_APPLICATION" \
        --options runtime \
        --timestamp \
        --entitlements "$entitlements" \
        --force \
        "$path"
}

if [[ -d "$TARGET" && "$TARGET" == *.app ]]; then
    ENTITLEMENTS="$TARGET/Contents/Resources/entitlements.plist"
    [[ -f "$ENTITLEMENTS" ]] || ENTITLEMENTS="$ENTITLEMENTS_DEFAULT"

    # Sign every Mach-O inside Contents/MacOS/ (main executable + any helpers
    # such as the embedded gosaid daemon) before sealing the bundle.
    while IFS= read -r -d '' exe; do
        sign_file "$exe" "$ENTITLEMENTS"
    done < <(find "$TARGET/Contents/MacOS" -type f -print0)

    sign_file "$TARGET" "$ENTITLEMENTS"
    codesign --verify --deep --strict --verbose=2 "$TARGET"
else
    sign_file "$TARGET" "$ENTITLEMENTS_DEFAULT"
    codesign --verify --verbose=2 "$TARGET"
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
ZIP="$WORK/notarize.zip"

echo "sign-notarize: zipping for notarization"
ditto -c -k --keepParent "$TARGET" "$ZIP"

echo "sign-notarize: submitting to Apple notary"
xcrun notarytool submit "$ZIP" \
    --apple-id "$NOTARY_APPLE_ID" \
    --team-id "$NOTARY_TEAM_ID" \
    --password "$NOTARY_PASSWORD" \
    --wait

# Staple the ticket so the bundle validates offline.  Stapling fails for
# raw Mach-O binaries, so only run it on bundles.
if [[ -d "$TARGET" && "$TARGET" == *.app ]]; then
    echo "sign-notarize: stapling notarization ticket"
    xcrun stapler staple "$TARGET"
fi

echo "sign-notarize: done"
