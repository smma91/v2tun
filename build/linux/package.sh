#!/usr/bin/env bash
# Builds the Linux release files into dist/:
#   v2tun-<version>-linux-amd64.tar.gz, .deb, .rpm and .AppImage
# Usage (from the repository root): build/linux/package.sh 1.0.0
# Needs: everything build.sh needs, plus nfpm (go install
# github.com/goreleaser/nfpm/v2/cmd/nfpm@latest) and appimagetool.
set -euo pipefail
cd "$(dirname "$0")/../.."
VERSION="${1:?version, e.g. 1.0.0}"
NAME="v2tun-$VERSION-linux-amd64"

./build.sh "$VERSION"
rm -rf dist && mkdir -p dist/stage
cp v2tun libcronet.so dist/stage/

# tar.gz: the program plus an installer script
T="dist/$NAME"
mkdir -p "$T"
cp v2tun libcronet.so build/linux/install.sh build/linux/v2tun.desktop LICENSE THIRD_PARTY_NOTICES.md README.md "$T/"
cp cmd/v2tun/frontend/icon.png "$T/v2tun.png"
chmod +x "$T/install.sh"
tar -C dist -czf "dist/$NAME.tar.gz" "$NAME"
rm -rf "$T"

# .deb / .rpm
export VERSION
nfpm package -f build/linux/nfpm.yaml -p deb -t "dist/v2tun_${VERSION}_amd64.deb"
nfpm package -f build/linux/nfpm.yaml -p rpm -t "dist/v2tun-${VERSION}-1.x86_64.rpm"

# AppImage: on first start it offers to install itself to /opt/v2tun with the
# network permission (capabilities do not work inside an AppImage).
A=dist/AppDir
mkdir -p "$A/usr/bin"
cp v2tun libcronet.so "$A/usr/bin/"
install -m 0755 build/linux/AppRun "$A/AppRun"
sed 's|^Exec=.*|Exec=v2tun|' build/linux/v2tun.desktop > "$A/v2tun.desktop"
cp cmd/v2tun/frontend/icon.png "$A/v2tun.png"
ARCH=x86_64 appimagetool --no-appstream "$A" "dist/$NAME.AppImage"
rm -rf "$A" dist/stage

(cd dist && sha256sum ./* > SHA256SUMS-linux.txt)
ls -la dist
