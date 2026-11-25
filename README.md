<h1 align="center">v2tun</h1>

<p align="center">
  <b>One click, and every app on your computer goes through your proxy, with a real kill switch.</b><br>
  System-wide tunnel for <b>Windows</b> and <b>Linux</b>, with a built-in core or your existing v2rayN.
</p>

<p align="center">
  <a href="https://github.com/smma91/v2tun/releases/latest"><img src="https://img.shields.io/github/v/release/smma91/v2tun?label=download&color=2563eb" alt="Latest release"></a>
  <img src="https://img.shields.io/badge/platform-Windows%20%7C%20Linux-0f172a" alt="Windows | Linux">
  <a href="https://github.com/smma91/v2tun/releases"><img src="https://img.shields.io/github/downloads/smma91/v2tun/total?color=16a34a" alt="Downloads"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/smma91/v2tun?color=64748b" alt="License"></a>
</p>

<p align="center">
  <b>English</b> | <a href="docs/README.fa.md">فارسی</a> | <a href="docs/README.ar.md">العربية</a> | <a href="docs/README.zh.md">中文</a>
</p>

<p align="center">
  <img src="docs/screenshots/connect.png" width="260" alt="Connect">
  <img src="docs/screenshots/servers.png" width="260" alt="Servers">
  <img src="docs/screenshots/settings.png" width="260" alt="Settings">
</p>

## Why v2tun?

| | |
|---|---|
| 🌐 **Every app, not just the browser** | Games, messengers, updaters, the terminal: all of them use the tunnel, with no per-app proxy settings. |
| 🛡️ **Nothing leaks** | Kill switch, IPv6 and DNS leak guard, and remote DNS: the leak tests show only your server. |
| 📶 **Share it over Wi-Fi** | Turn your PC into a hotspot. Phones and laptops that join it are tunneled too. |
| ⚡ **All modern protocols** | VLESS Reality / XHTTP, VMess, Trojan, Shadowsocks, Hysteria2, TUIC, WireGuard, NaiveProxy, OpenVPN, AnyConnect and more. |
| 🚀 **Always the fastest server** | Ping, real delay and speed test, plus "Auto" mode that switches servers without dropping your downloads. |
| 🔌 **Works with v2rayN** | Already use v2rayN? v2tun finds it and tunnels its proxy for the whole system. |
| 🧩 **Simple and free** | One small program, a clean dark interface in 4 languages, open source (GPL-3.0). |

## Features

### 🛡️ Privacy and protection
- **System-wide TUN tunnel** for all apps (Wintun on Windows, `/dev/net/tun` on Linux).
- **Kill switch** (Windows Filtering Platform / nftables): if the tunnel or the proxy drops, the internet stays blocked instead of falling back to your real connection.
- **IPv6 and DNS leak guard**, also when the kill switch is off.
- **Remote DNS (FakeIP):** DNS queries never leave your computer. The server resolves names in the exit country, so DNS leak tests come out clean.
- **Built-in leak test** (IP and WebRTC) and exit IP display with country and provider.
- **Time zone matching** (optional): the system time zone follows the server's country.
- **Crash safe:** a watchdog restores the network, the time zone and the proxy settings even if v2tun is killed.

### ⚡ Servers and protocols
- **Built-in [sing-box](https://github.com/SagerNet/sing-box) core:** VLESS (Reality, XHTTP via Xray), VMess, Trojan, Shadowsocks, Hysteria / Hysteria2, TUIC, WireGuard, NaiveProxy, SOCKS, HTTP, OpenVPN (`.ovpn`), AnyConnect / OpenConnect / Fortinet / GlobalProtect.
- **Subscriptions:** base64 links, sing-box and Clash formats, with auto-update.
- **Ping, real delay and speed test**, for one server or all of them.
- **Auto (fastest)** with failover that keeps open connections alive.
- **v2rayN mode:** use the servers you already have in v2rayN.

### 📶 Sharing
- **Wi-Fi hotspot:** phones, tablets and laptops that join it use the tunnel, with the same protection. If the tunnel drops, they have no internet instead of a leak. It uses Windows Mobile Hotspot (or Wi-Fi Direct) on Windows and NetworkManager on Linux.
- **Local proxy:** SOCKS5 and HTTP on one port, optionally reachable from your network.
- **"Proxy only" mode:** no tunnel, just the local proxy, set as the system proxy.

### 🧭 Control
- **Split tunneling** by app and by domain / IP range, in "exclude" or "include" mode.
- **Smart QUIC handling:** when your server cannot carry UDP, apps switch to TCP at once instead of hanging.

### 🖥️ Desktop
- Tray icon, start at login (no password or UAC prompt), auto-connect.
- Interface in **English, Persian, Arabic and Chinese**.
- Windows 10/11 and Linux (Ubuntu, Debian, Mint, Fedora, AppImage).

## Download

Get the latest version from **[Releases](https://github.com/smma91/v2tun/releases/latest)**. Installation for each system is described below.

## Windows

**Requirements:** Windows 10 or 11 (x64), administrator rights (for the
tunnel adapter, routes and firewall) and the
[WebView2 runtime](https://developer.microsoft.com/microsoft-edge/webview2/)
(included in Windows 11 and up-to-date Windows 10).

1. Download `v2tun-<version>-windows-amd64.zip` from
   [Releases](../../releases) and check it against `SHA256SUMS.txt`.
2. Unzip it anywhere and run `v2tun.exe` (it asks for administrator rights).

Settings, servers and the log are stored in `C:\ProgramData\v2tun`
(readable only by administrators).

> **Antivirus warning.** v2tun changes the firewall, routes, DNS and the
> hosts file and contains a network driver loader. Unsigned builds of such
> tools are sometimes flagged as false positives. You can always build the
> exe yourself from this source (see below).

## Linux

**Requirements:** a 64-bit (x86_64) distribution with WebKitGTK 4.1, for
example Ubuntu 22.04+, Debian 12+, Linux Mint 21+ or Fedora 37+.

Download from [Releases](../../releases) and check against `SHA256SUMS.txt`:

| Package | Install |
|---|---|
| `.deb` (Ubuntu, Debian, Mint) | `sudo apt install ./v2tun_<version>_amd64.deb` |
| `.rpm` (Fedora) | `sudo dnf install ./v2tun-<version>-1.x86_64.rpm` |
| `.AppImage` (any) | `chmod +x v2tun-*.AppImage` and run it |
| `.tar.gz` (any) | unpack, then `sudo ./install.sh` |

Then start **v2tun** from the applications menu (or run `v2tun`).

v2tun does not run as root: its window runs as your user, and the program
gets only the two network capabilities it needs (`cap_net_admin`,
`cap_net_raw`). The packages and `install.sh` set them; when they are
missing (AppImage, or a copied binary), v2tun shows a **Grant permission**
button that asks for your password once. The AppImage then installs itself to
`/opt/v2tun`, because capabilities do not work inside an AppImage.

Notes:
- The tray icon needs a tray: KDE, Xfce, Cinnamon and Ubuntu work out of the
  box; on plain GNOME (e.g. Fedora) install the *AppIndicator* extension.
  Without a tray, closing the window minimises it.
- "Proxy only" sets the system proxy on GNOME-based desktops and KDE.
- Settings, servers and the log are in `~/.config/v2tun` (mode 0700).
- Uninstall: remove the package, or `sudo ./install.sh --uninstall`.

## Usage

1. Either add servers on the **Servers** tab (paste a link or subscription),
   or choose **v2rayN** as the proxy source while v2rayN is running.
2. Press the power button.

## Build from source

**Windows** — Go 1.26 (the `go` command downloads the exact toolchain
automatically) and PowerShell; no C compiler is needed:

```powershell
git clone https://github.com/smma91/v2tun.git
cd v2tun
.\build.ps1                 # or: .\build.ps1 -Version 1.0.0
```

The result is a single `v2tun.exe`; `wintun.dll` and `libcronet.dll` are
embedded in it.

**Linux** — Go 1.26, a C compiler and the GTK / WebKitGTK development files:

```sh
sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev   # Fedora: gcc gtk3-devel webkit2gtk4.1-devel
./build.sh                  # or: ./build.sh 1.0.0
sudo setcap cap_net_admin,cap_net_raw+ep ./v2tun
```

The result is `v2tun` plus `libcronet.so` (NaiveProxy), which must stay next to
it. `build/linux/package.sh <version>` makes the `.deb`, `.rpm`, AppImage and
`tar.gz` (needs [nfpm](https://nfpm.goreleaser.com) and
[appimagetool](https://github.com/AppImage/appimagetool)).

### Tests

```powershell
$tags = 'desktop,production,with_quic,with_utls,with_wireguard,with_gvisor,with_naive_outbound,with_purego,with_openvpn,with_openconnect'
Rename-Item cmd/v2tun/rsrc_windows_amd64.syso rsrc.syso.bak   # Windows: the icon resource clashes with test binaries
go test -tags $tags -run 'TestT$|TestRules|TestSniff|TestFakeIP|TestParseLinks|TestNewFormats|TestHostsStrip' ./cmd/v2tun
Rename-Item cmd/v2tun/rsrc.syso.bak rsrc_windows_amd64.syso
```

On Linux add `webkit2_41` to the tags. End-to-end tests that really bring the
tunnel up (`V2TUN_E2E=1`, as Administrator / root) and the manual release
checklist are described in [docs/TESTING.md](docs/TESTING.md).

### Translations

All text in the code is English and is translated through
[`cmd/v2tun/frontend/i18n.json`](cmd/v2tun/frontend/i18n.json). `node tools/extract.js` lists
strings that have no translation yet; add them to `tools/tr-extra.json`
(`[["English", "Persian", "Chinese", "Arabic"], …]`) and run
`node tools/mergei18n.js`.

## Project layout

```
cmd/v2tun/            the application (Go, package main)
  ├─ frontend/        UI (plain HTML / CSS / JS) and translations (i18n.json)
  └─ assets/          embedded files: wintun.dll, libcronet.dll, tray icons
build/windows/        app icon and manifest for the Windows resource
build/linux/          Linux packaging: .deb/.rpm (nfpm), AppImage, tar.gz installer
docs/                 test checklist, release guide, translations of this README, screenshots
tools/                icon generator, translation and license-notice helpers
.github/              CI build / release workflow, issue templates
```

Inside `cmd/v2tun`, shared code: `engine.go` / `handler.go` / `dialer.go`
(tunnel and per-connection routing), `fakeip.go` / `dnstcp.go` (DNS),
`core.go` / `servers.go` / `links.go` / `clash.go` / `ovpn.go` (built-in core
and server parsing), `rules.go` (split tunneling). System-specific code is in
`*_windows.go` / `*_linux.go` files: firewall (WFP / nftables), TUN device,
routes, process lookup, system proxy, time zone, autostart and the watchdog.

## Releasing

See [docs/RELEASE.md](docs/RELEASE.md). Pushing a tag such as `v1.0.0` makes the GitHub
Actions workflow build both platforms and create a draft release with all
files and checksums.

## Feedback

Found a bug or have an idea? Please [open an issue](../../issues/new/choose).
Remove server addresses, passwords and subscription links from logs before
posting them. Pull requests are not accepted at the moment.

## ☕ Support the project

v2tun is free and open source. If it helps you, you can buy me a coffee:

| Network | Address |
|---|---|
| Ethereum, BNB Smart Chain, Polygon, Arbitrum and other EVM chains (USDT, USDC, ETH, BNB, …) | `0x1404A760e5A30f9DD1D3AC68726673f94DC2f649` |

Please double-check the address and the network before you send. Stars ⭐ and bug reports help a lot too.

## License

v2tun is licensed under the [GNU General Public License v3.0](LICENSE)
because it includes sing-box and other GPL-licensed libraries. Bundled
third-party components and their licenses are listed in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

This software is provided as is, without warranty. You are responsible for
using it in accordance with the laws of your country and the terms of your
proxy provider.
