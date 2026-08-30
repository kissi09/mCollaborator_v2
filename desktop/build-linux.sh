#!/usr/bin/env bash
#
# Builds the mCollaborator desktop app for Linux and packages it as a .deb.
#
# This is build.ps1's counterpart and does the same three things in the same
# order: build the backend server and stage it beside the shell, regenerate the
# icon, then build the app and package it. Everything lands in dist/.
#
# It must run ON Linux. The shell is a Wails app, and Wails' Linux window is cgo
# against GTK and WebKit, so it cannot be cross-compiled from Windows or macOS -
# `go build` without -tags production will happily produce a binary anyway, but
# that binary contains Wails' no-op default frontend and opens no window at all.
# See ../.github/workflows/desktop-release.yml for building this without a Linux
# machine to hand.
#
# Build dependencies (Debian/Ubuntu):
#   sudo apt install build-essential pkg-config libgtk-3-dev dpkg-dev \
#                    libwebkit2gtk-4.1-dev     # or libwebkit2gtk-4.0-dev on older releases
#
# Usage:
#   ./build-linux.sh                 the current architecture, .deb included
#   ./build-linux.sh --skip-server   reuse the staged server binary
#   ./build-linux.sh --no-deb        the binaries only

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
root="$PWD"
backend="$(cd .. && pwd)/backend"
dist="$root/dist"
bin_dir="$root/build/bin"

skip_server=0
make_deb=1

while [[ $# -gt 0 ]]; do
    case "$1" in
        --skip-server) skip_server=1; shift ;;
        --no-deb) make_deb=0; shift ;;
        -h|--help) sed -n '2,23p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
done

step() { printf '\n==> %s\n' "$1"; }
note() { printf '    %s\n' "$1"; }
die()  { printf '\nerror: %s\n' "$1" >&2; exit 1; }

[[ "$(uname -s)" == "Linux" ]] || die "this builds a Linux app and has to run on Linux (see the header)"
command -v go >/dev/null || die "go is not installed"
command -v pkg-config >/dev/null || die "pkg-config is not installed (see the header)"

export PATH="$PATH:$(go env GOPATH)/bin"
command -v wails >/dev/null ||
    die "wails is not installed; run: go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0"

# Debian's architecture names are not Go's.
goarch="$(go env GOARCH)"
case "$goarch" in
    amd64) debarch="amd64" ;;
    arm64) debarch="arm64" ;;
    *) die "unsupported architecture: $goarch" ;;
esac

# --- which WebKit ----------------------------------------------------------
#
# Wails links webkit2gtk-4.0 by default and 4.1 behind a build tag. Ubuntu 24.04
# and Debian 13 ship only 4.1, older releases only 4.0, and the package the .deb
# has to depend on is whichever one it was actually built against. Getting this
# wrong produces a .deb that installs cleanly and then fails to start with a
# missing shared library, so it is detected rather than assumed.
if pkg-config --exists webkit2gtk-4.1; then
    webkit_tags="webkit2_41"
    webkit_dep="libwebkit2gtk-4.1-0"
elif pkg-config --exists webkit2gtk-4.0; then
    webkit_tags=""
    webkit_dep="libwebkit2gtk-4.0-37"
else
    die "no webkit2gtk development files found; install libwebkit2gtk-4.1-dev (see the header)"
fi
note "building against ${webkit_dep%-*}, depending on $webkit_dep"

mkdir -p "$dist" "$bin_dir"

# --- 1. the server ---------------------------------------------------------
#
# The backend is pure Go - its SQLite driver is modernc.org/sqlite, not a cgo
# one - so it needs no toolchain and no shared libraries of its own.
server_out="$bin_dir/mCollaborator-server"
if [[ $skip_server -eq 0 ]]; then
    step "Building the mCollaborator server ($goarch)"
    ( cd "$backend" && CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" \
        go build -trimpath -ldflags '-s -w' -o "$server_out" . )
    note "staged $server_out"
else
    step "Reusing the staged server binary"
    [[ -x "$server_out" ]] || die "no staged server at $server_out; run without --skip-server"
fi

# --- 2. the icon -----------------------------------------------------------
#
# The .ico the same tool produces is a Windows artefact and goes to a throwaway
# path, so a build here does not show up as a change to a committed Windows
# asset. build/appicon.png is what both Wails and the .desktop entry use.
step "Generating the application icon"
go run ./tools/mkicon \
    -mark ../backend/static/images/cyberteq-mark.png \
    -reference ../backend/static/apple-touch-icon.png \
    -out "$(mktemp)" \
    -png build/appicon.png

# --- 3. the app ------------------------------------------------------------
step "Building the app ($goarch)"
if [[ -n "$webkit_tags" ]]; then
    wails build -platform "linux/$goarch" -skipbindings -clean -tags "$webkit_tags"
else
    wails build -platform "linux/$goarch" -skipbindings -clean
fi

app="$bin_dir/mCollaborator"
[[ -f "$app" ]] || die "wails did not produce $app"
note "$(du -h "$app" | cut -f1)"

# --- 4. the .deb -----------------------------------------------------------
version="$(python3 - <<'PY' 2>/dev/null || echo 3.0.0
import json
print(json.load(open("wails.json"))["info"]["productVersion"].rsplit(".", 1)[0])
PY
)"
deb="$dist/mcollaborator_${version}_${debarch}.deb"

if [[ $make_deb -eq 1 ]]; then
    command -v dpkg-deb >/dev/null || die "dpkg-deb is not installed (apt install dpkg-dev)"
    step "Building $(basename "$deb")"

    pkg="$(mktemp -d)"
    trap 'rm -rf "$pkg"' EXIT

    # Both executables go in one private directory and the launcher in
    # /usr/bin is a symlink to the shell. The shell finds its server as a
    # sibling of its own executable, and on Linux os.Executable() reads
    # /proc/self/exe, which resolves the symlink - so the lookup lands in
    # /usr/lib/mcollaborator, where the server actually is.
    install -Dm755 "$app"        "$pkg/usr/lib/mcollaborator/mCollaborator"
    install -Dm755 "$server_out" "$pkg/usr/lib/mcollaborator/mCollaborator-server"
    install -d "$pkg/usr/bin"
    ln -s /usr/lib/mcollaborator/mCollaborator "$pkg/usr/bin/mcollaborator"

    install -Dm644 build/appicon.png \
        "$pkg/usr/share/icons/hicolor/256x256/apps/mcollaborator.png"

    install -Dm644 /dev/stdin "$pkg/usr/share/applications/mcollaborator.desktop" <<'EOF'
[Desktop Entry]
Type=Application
Name=mCollaborator
GenericName=VAPT Reporting
Comment=Vulnerability assessment and penetration testing reporting
Exec=mcollaborator
Icon=mcollaborator
Terminal=false
Categories=Office;Security;
StartupWMClass=mCollaborator
EOF

    installed_kb="$(du -sk "$pkg" | cut -f1)"
    install -Dm644 /dev/stdin "$pkg/DEBIAN/control" <<EOF
Package: mcollaborator
Version: $version
Section: utils
Priority: optional
Architecture: $debarch
Depends: libgtk-3-0, $webkit_dep
Maintainer: Cyberteq Falcon Ltd. <info@cyberteq.com>
Installed-Size: $installed_kb
Description: VAPT reporting for Cyberteq Falcon Ltd.
 mCollaborator builds vulnerability assessment and penetration testing
 reports from an engagement: a Word report, its PDF, and the closing
 meeting deck.
 .
 This package installs the desktop application, which runs the
 mCollaborator server on a private loopback port and shows it in its own
 window. Engagement data is kept per-user under ~/.config/mCollaborator
 and never in the install directory.
EOF

    # A stale ~/.config/mCollaborator from a previous install is the user's
    # data, so removal deliberately leaves it alone; only the icon cache is
    # worth refreshing, and only when the tool for it exists.
    install -Dm755 /dev/stdin "$pkg/DEBIAN/postinst" <<'EOF'
#!/bin/sh
set -e
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -q -t -f /usr/share/icons/hicolor || true
fi
if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database -q /usr/share/applications || true
fi
EOF
    cp "$pkg/DEBIAN/postinst" "$pkg/DEBIAN/postrm"

    rm -f "$deb"
    dpkg-deb --build --root-owner-group "$pkg" "$deb" >/dev/null
    note "$(basename "$deb")  $(du -h "$deb" | cut -f1)"

    # dpkg-deb builds almost anything; lintian is what says whether it is a
    # well-formed package. It is advisory here - a warning is not worth failing
    # a build over - but silence about it would be worse.
    if command -v lintian >/dev/null; then
        lintian --no-tag-display-limit "$deb" || note "lintian had complaints (above); the package is still usable"
    fi
fi

# --- collect ---------------------------------------------------------------
step "Collecting artefacts into dist/"
cp "$app" "$dist/mCollaborator"
cp "$server_out" "$dist/mCollaborator-server"
for f in "$dist"/mCollaborator "$dist"/mCollaborator-server "$deb"; do
    [[ -e "$f" ]] && printf '    %-40s %8s\n' "$(basename "$f")" "$(du -h "$f" | cut -f1)"
done

cat <<EOF

Done. Artefacts are in $dist

  mCollaborator              the desktop app
  mCollaborator-server       the server it runs; both must sit together
  $(basename "$deb")   what you hand someone else

Install it with:

  sudo apt install ./$(basename "$deb")

EOF
