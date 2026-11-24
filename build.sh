#!/usr/bin/env bash
# Builds the Linux version: ./v2tun (and copies libcronet.so next to it).
#   ./build.sh                 -> version from app.go
#   ./build.sh 1.0.0           -> release build
#
# Needs Go 1.26, a C compiler, pkg-config and the GTK 3 / WebKitGTK 4.1
# development files:
#   Debian/Ubuntu: sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
#   Fedora:        sudo dnf install gcc pkgconf-pkg-config gtk3-devel webkit2gtk4.1-devel
set -euo pipefail
cd "$(dirname "$0")"
VERSION="${1:-}"
# Same protocols as on Windows; webkit2_41 selects WebKitGTK 4.1.
TAGS='desktop,production,webkit2_41,with_quic,with_utls,with_wireguard,with_gvisor,with_naive_outbound,with_purego,with_openvpn,with_openconnect'
LDFLAGS='-s -w'
if [ -n "$VERSION" ]; then LDFLAGS="$LDFLAGS -X main.appVersion=$VERSION"; fi
CGO_ENABLED=1 go build -buildvcs=false -tags "$TAGS" -trimpath -ldflags "$LDFLAGS" -o v2tun ./cmd/v2tun
# NaiveProxy loads Chromium's network stack at run time from the program's folder.
CRONET="$(go list -m -f '{{.Dir}}' github.com/sagernet/cronet-go/lib/linux_amd64)/libcronet.so"
install -m 0644 "$CRONET" libcronet.so
echo "built: $(pwd)/v2tun"
