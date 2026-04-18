#!/usr/bin/env bash
# Render Formula/gosaid.rb for the homebrew-tap repo and push it.
# Expects all release archives + checksums.txt in $ARTIFACTS_DIR.
#
# Required env vars:
#   VERSION             e.g. 0.1.0 (no leading "v")
#   ARTIFACTS_DIR       directory containing gosaid-${VERSION}-*.tar.gz and checksums.txt
#   TAP_REPO            e.g. dmtrkzntsv/homebrew-tap
#   TAP_GITHUB_TOKEN    PAT with contents:write on TAP_REPO
#   GITHUB_REPOSITORY   e.g. dmtrkzntsv/gosaid (used to build download URLs)

set -euo pipefail

: "${VERSION:?required}"
: "${ARTIFACTS_DIR:?required}"
: "${TAP_REPO:?required}"
: "${TAP_GITHUB_TOKEN:?required}"
: "${GITHUB_REPOSITORY:?required}"

cd "$ARTIFACTS_DIR"

sha_for() {
    local archive="gosaid-${VERSION}-$1.tar.gz"
    if [[ ! -f "$archive" ]]; then
        echo "missing archive: $archive" >&2
        exit 1
    fi
    awk -v f="$archive" '$2 == f { print $1 }' checksums.txt
}

SHA_DARWIN_ARM64=$(sha_for darwin-arm64)
SHA_DARWIN_AMD64=$(sha_for darwin-amd64)
SHA_LINUX_AMD64=$(sha_for linux-amd64)
SHA_LINUX_ARM64=$(sha_for linux-arm64)

BASE_URL="https://github.com/${GITHUB_REPOSITORY}/releases/download/v${VERSION}"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

git clone "https://x-access-token:${TAP_GITHUB_TOKEN}@github.com/${TAP_REPO}.git" "$WORK/tap"
mkdir -p "$WORK/tap/Formula"

cat > "$WORK/tap/Formula/gosaid.rb" <<EOF
class Gosaid < Formula
  desc "Headless cross-platform push-to-talk voice dictation daemon"
  homepage "https://github.com/${GITHUB_REPOSITORY}"
  version "${VERSION}"
  license "MIT"

  on_macos do
    on_arm do
      url "${BASE_URL}/gosaid-${VERSION}-darwin-arm64.tar.gz"
      sha256 "${SHA_DARWIN_ARM64}"
    end
    on_intel do
      url "${BASE_URL}/gosaid-${VERSION}-darwin-amd64.tar.gz"
      sha256 "${SHA_DARWIN_AMD64}"
    end
  end

  on_linux do
    on_arm do
      url "${BASE_URL}/gosaid-${VERSION}-linux-arm64.tar.gz"
      sha256 "${SHA_LINUX_ARM64}"
    end
    on_intel do
      url "${BASE_URL}/gosaid-${VERSION}-linux-amd64.tar.gz"
      sha256 "${SHA_LINUX_AMD64}"
    end
  end

  def install
    bin.install "gosaid"
  end

  service do
    run [opt_bin/"gosaid"]
    keep_alive true
    log_path var/"log/gosaid.log"
    error_log_path var/"log/gosaid.log"
  end

  def caveats
    <<~EOS
      Configure: gosaid config
      Run in background: brew services start gosaid

      macOS: grant Accessibility (for global hotkeys + paste) and Microphone
      on first use in System Settings -> Privacy & Security.

      Linux: install a keystroke-injection tool (wtype / xdotool / ydotool).
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/gosaid version")
  end
end
EOF

cd "$WORK/tap"
git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git add Formula/gosaid.rb
if git diff --cached --quiet; then
    echo "update-brew-formula: no changes to formula, skipping push"
    exit 0
fi
git commit -m "gosaid ${VERSION}"
git push origin HEAD

echo "update-brew-formula: pushed Formula/gosaid.rb to ${TAP_REPO}"
