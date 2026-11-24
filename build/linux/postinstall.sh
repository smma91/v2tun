#!/bin/sh
# v2tun creates a TUN interface, routes and firewall rules. Instead of running
# as root it gets just these two capabilities, so its window runs as the user.
setcap cap_net_admin,cap_net_raw+ep /opt/v2tun/v2tun || \
  echo "v2tun: setcap failed; run: sudo setcap cap_net_admin,cap_net_raw+ep /opt/v2tun/v2tun" >&2
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database -q /usr/share/applications || true
command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -q -t /usr/share/icons/hicolor || true
exit 0
