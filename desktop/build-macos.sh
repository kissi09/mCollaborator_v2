#!/usr/bin/env bash
#
# Builds mCollaborator.app and a .dmg to distribute it in.
#
# This is build.ps1's counterpart and does the same three things in the same
# order: build the backend server and stage it beside the shell, regenerate the
# icon, then build the app and package it. Everything lands in dist/.
#
# It must run ON a Mac. The shell is a Wails app, and Wails' macOS window is
# cgo against WebKit, so it cannot be cross-compiled from Windows or Linux -
# `go build` without -tags production will happily produce a binary anyway, but
# that binary contains Wails' no-op default frontend and opens no window at all.
# See ci/desktop-release.yml for building this without a Mac to hand - and
# ci/README.md for why that file is parked there rather than being live.
#
# Usage:
#   ./build-macos.sh                 universal (arm64 + x86_64), unsigned
#   ./build-macos.sh --arch arm64    this machine's architecture only
#   ./build-macos.sh --skip-server   reuse the staged server binary
#   ./build-macos.sh --no-dmg        the .app only
#
# Signing, all optional and read from the environment:
#   MACOS_SIGN_IDENTITY   e.g. "Developer ID Application: Cyberteq Falcon Ltd. (TEAMID)"
#   MACOS_NOTARY_PROFILE  a `xcrun notarytool store-credentials` profile name
#
# Unsigned is fine for internal use but Gatekeeper will refuse the app on a
# machine that did not build it; the README says what the recipient has to do.

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
root="$PWD"
backend="$(cd .. && pwd)/backend"
dist="$root/dist"
bin_dir="$root/build/bin"

arch="universal"
skip_server=0
make_dmg=1

while [[ $# -gt 0 ]]; do
    case "$1" in
        --arch) arch="$2"; shift 2 ;;
        --skip-server) skip_server=1; shift ;;
        --no-dmg) make_dmg=0; shift ;;
        -h|--help) sed -n '2,30p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
done

step() { printf '\n==> %s\n' "$1"; }
note() { printf '    %s\n' "$1"; }
die()  { printf '\nerror: %s\n' "$1" >&2; exit 1; }

[[ "$(uname -s)" == "Darwin" ]] || die "this builds a macOS app and has to run on macOS (see the header)"
command -v go >/dev/null || die "go is not installed"

# Wails installs into GOPATH/bin, which is not always on PATH.
export PATH="$PATH:$(go env GOPATH)/bin"
command -v wails >/dev/null ||
    die "wails is not installed; run: go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0"

mkdir -p "$dist" "$bin_dir"

# --- 1. the server ---------------------------------------------------------
#
# The backend is pure Go - its SQLite driver is modernc.org/sqlite, not a cgo
# one - so it cross-compiles per architecture with no toolchain, and the two
# halves of a universal app are made by building both and lipo-ing them.
server_out="$bin_dir/mCollaborator-server"
if [[ $skip_server -eq 0 ]]; then
    step "Building the mCollaborator server ($arch)"
    build_server() { # $1 = GOARCH, $2 = output
        ( cd "$backend" && CGO_ENABLED=0 GOOS=darwin GOARCH="$1" \
            go build -trimpath -ldflags '-s -w' -o "$2" . )
    }
    case "$arch" in
        universal)
            build_server arm64 "$bin_dir/.server-arm64"
            build_server amd64 "$bin_dir/.server-amd64"
            lipo -create -output "$server_out" "$bin_dir/.server-arm64" "$bin_dir/.server-amd64"
            rm -f "$bin_dir/.server-arm64" "$bin_dir/.server-amd64"
            note "$(lipo -archs "$server_out")"
            ;;
        arm64|amd64) build_server "$arch" "$server_out" ;;
        *) die "unknown architecture: $arch (use universal, arm64 or amd64)" ;;
    esac
    note "staged $server_out"
else
    step "Reusing the staged server binary"
    [[ -x "$server_out" ]] || die "no staged server at $server_out; run without --skip-server"
fi

# --- 2. the icon -----------------------------------------------------------
#
# Wails takes build/appicon.png for the macOS bundle icon. The .ico the same
# tool produces is a Windows artefact and is written to a throwaway path here,
# so a build on this platform does not show up as a change to a committed
# Windows asset.
step "Generating the application icon"
go run ./tools/mkicon \
    -mark ../backend/static/images/cyberteq-mark.png \
    -reference ../backend/static/apple-touch-icon.png \
    -out "$(mktemp)" \
    -png build/appicon.png

# --- 3. the app ------------------------------------------------------------
step "Building mCollaborator.app ($arch)"
wails build -platform "darwin/$arch" -skipbindings -clean

app="$bin_dir/mCollaborator.app"
[[ -d "$app" ]] || die "wails did not produce $app"

# The shell looks for its server beside its own executable, which inside a
# bundle is Contents/MacOS. This is the step that makes the .app self-contained;
# without it the app launches, finds nothing, and shows its startup failure.
step "Staging the server inside the bundle"
cp "$server_out" "$app/Contents/MacOS/mCollaborator-server"
chmod +x "$app/Contents/MacOS/mCollaborator-server"
note "Contents/MacOS/mCollaborator-server"

# --- 4. signing ------------------------------------------------------------
#
# Signed inside-out: the nested binary first, then the bundle, or the outer
# signature is invalidated by the inner one being added afterwards.
if [[ -n "${MACOS_SIGN_IDENTITY:-}" ]]; then
    step "Signing"
    codesign --force --options runtime --timestamp \
        --sign "$MACOS_SIGN_IDENTITY" "$app/Contents/MacOS/mCollaborator-server"
    codesign --force --options runtime --timestamp --deep \
        --sign "$MACOS_SIGN_IDENTITY" "$app"
    codesign --verify --strict --verbose=2 "$app"
    note "signed as $MACOS_SIGN_IDENTITY"
else
    step "Not signing"
    note "MACOS_SIGN_IDENTITY is unset, so this build is unsigned."
    note "Gatekeeper will refuse it on any Mac other than this one."
fi

# --- 5. the disk image -----------------------------------------------------
version="$(python3 - <<'PY' 2>/dev/null || echo 3.0.0
import json
print(json.load(open("wails.json"))["info"]["productVersion"].rsplit(".", 1)[0])
PY
)"
dmg="$dist/mCollaborator-$version-$arch.dmg"

if [[ $make_dmg -eq 1 ]]; then
    step "Building the disk image"
    staging="$(mktemp -d)"
    trap 'rm -rf "$staging"' EXIT
    cp -R "$app" "$staging/"
    # The Applications symlink is what makes the window a drag-and-drop
    # install rather than a folder the user has to know what to do with.
    ln -s /Applications "$staging/Applications"

    rm -f "$dmg"
    hdiutil create -volname "mCollaborator" -srcfolder "$staging" \
        -ov -format UDZO "$dmg" >/dev/null
    note "$(basename "$dmg")  $(du -h "$dmg" | cut -f1)"

    if [[ -n "${MACOS_NOTARY_PROFILE:-}" ]]; then
        step "Notarising"
        xcrun notarytool submit "$dmg" --keychain-profile "$MACOS_NOTARY_PROFILE" --wait
        xcrun stapler staple "$dmg"
        note "notarised and stapled"
    elif [[ -n "${MACOS_SIGN_IDENTITY:-}" ]]; then
        note "MACOS_NOTARY_PROFILE is unset, so the image is signed but not notarised."
    fi
fi

# --- collect ---------------------------------------------------------------
step "Collecting artefacts into dist/"
rm -rf "$dist/mCollaborator.app"
cp -R "$app" "$dist/"
cp "$server_out" "$dist/mCollaborator-server"
for f in "$dist"/mCollaborator*; do
    printf '    %-40s %8s\n' "$(basename "$f")" "$(du -sh "$f" | cut -f1)"
done

cat <<EOF

Done. Artefacts are in $dist

  mCollaborator.app          the app; the server is inside it, in Contents/MacOS
  mCollaborator-server       the same server on its own, for a headless install
  mCollaborator-*.dmg        what you hand someone else

EOF

if [[ -z "${MACOS_SIGN_IDENTITY:-}" ]]; then
cat <<'EOF'
This build is unsigned. On another Mac, opening it gives "mCollaborator is
damaged and can't be opened" - which is Gatekeeper's message for unsigned, not
a corrupt download. The recipient clears the quarantine flag with:

  xattr -dr com.apple.quarantine /Applications/mCollaborator.app

Set MACOS_SIGN_IDENTITY and MACOS_NOTARY_PROFILE to stop them having to.

EOF
fi
