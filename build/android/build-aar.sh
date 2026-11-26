#!/usr/bin/env bash
# Builds android/app/libs/libv2tun.aar (v2tun's mobile package + sing-box's
# libbox) with gomobile. Needs Go 1.26, the Android SDK and NDK (ANDROID_HOME,
# ANDROID_NDK_HOME) and JDK 17+. Same as build-aar.ps1, for Linux / CI.
set -euo pipefail
cd "$(dirname "$0")/../.."
export PATH="$(go env GOPATH)/bin:$PATH"
command -v gomobile >/dev/null || {
  go install github.com/sagernet/gomobile/cmd/gomobile@v0.1.13
  go install github.com/sagernet/gomobile/cmd/gobind@v0.1.13
}
mkdir -p android/app/libs
# badlinkname / -checklinkname=0: libbox reaches into Go internals (as sing-box's
# own Android build does). NaiveProxy is left out (it needs a native library).
TAGS='with_gvisor,with_quic,with_wireguard,with_utls,with_openvpn,with_openconnect,badlinkname,tfogo_checklinkname0'
gomobile bind -v -target "${TARGETS:-android/arm64,android/arm,android/amd64}" -androidapi 21 \
  -javapkg io.github.smma91.v2tun -libname box -tags "$TAGS" -trimpath \
  -ldflags '-s -w -buildid= -checklinkname=0' \
  -o android/app/libs/libv2tun.aar ./mobile github.com/sagernet/sing-box/experimental/libbox
ls -la android/app/libs
