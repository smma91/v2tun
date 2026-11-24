#!/bin/sh
# Installs v2tun from this folder (the tar.gz) to /opt/v2tun, with a menu
# entry and the network capabilities. Run: sudo ./install.sh
# Remove again: sudo ./install.sh --uninstall
set -e
if [ "$(id -u)" != 0 ]; then
  echo "run with sudo: sudo $0 $*" >&2
  exit 1
fi
if [ "$1" = "--uninstall" ]; then
  rm -rf /opt/v2tun /usr/bin/v2tun /usr/share/applications/v2tun.desktop /usr/share/icons/hicolor/256x256/apps/v2tun.png
  echo "v2tun removed (your settings stay in ~/.config/v2tun)"
  exit 0
fi
cd "$(dirname "$0")"
install -d /opt/v2tun /usr/share/applications /usr/share/icons/hicolor/256x256/apps
install -m 0755 v2tun /opt/v2tun/v2tun
install -m 0644 libcronet.so /opt/v2tun/libcronet.so
install -m 0644 v2tun.desktop /usr/share/applications/v2tun.desktop
install -m 0644 v2tun.png /usr/share/icons/hicolor/256x256/apps/v2tun.png
ln -sf /opt/v2tun/v2tun /usr/bin/v2tun
setcap cap_net_admin,cap_net_raw+ep /opt/v2tun/v2tun
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database -q /usr/share/applications || true
echo "v2tun installed: start it from the applications menu or run 'v2tun'"
