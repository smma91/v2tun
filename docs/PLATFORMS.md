# Features per platform

Plan for the mobile versions: which desktop features make sense on each
platform. Legend: ✅ full · ⚠️ limited / different · ❌ not possible ·
➖ not needed · ➡️ done differently.

## 1. Tunnel and protection

| Feature | Windows | Linux | Android | Android TV | iOS | Notes |
|---|---|---|---|---|---|---|
| System-wide tunnel | ✅ | ✅ | ✅ | ✅ | ✅ | Android: VpnService (no root). iOS: Network Extension (packet tunnel) |
| Kill switch while connected | ✅ | ✅ | ✅ | ✅ | ✅ | |
| Kill switch after the app ends | ✅ | ✅ | ⚠️ | ⚠️ | ✅ | Android: the user enables "Always-on VPN" + "Block connections without VPN"; most TVs lack the setting. iOS: includeAllNetworks + On-Demand |
| IPv6 / DNS leak guard | ✅ | ✅ | ✅ | ✅ | ✅ | Part of the tunnel settings on mobile |
| Remote DNS (FakeIP) | ✅ | ✅ | ✅ | ✅ | ✅ | |
| Leak test, exit IP | ✅ | ✅ | ✅ | ✅ | ✅ | |
| Match the time zone | ✅ | ⚠️ | ❌ | ❌ | ❌ | Mobile apps may not change the system time zone |
| Crash restore (watchdog) | ✅ | ✅ | ➖ | ➖ | ➖ | The system removes the VPN itself |

## 2. Servers

| Feature | Windows | Linux | Android | Android TV | iOS | Notes |
|---|---|---|---|---|---|---|
| Built-in core, all protocols | ✅ | ✅ | ✅ | ✅ | ⚠️ | iOS: the tunnel process has a 50 MB memory limit |
| Subscriptions, auto-update | ✅ | ✅ | ✅ | ✅ | ✅ | |
| Add from clipboard | ✅ | ✅ | ✅ | ⚠️ | ✅ | TVs rarely have a clipboard |
| .ovpn files | ✅ | ✅ | ✅ | ⚠️ | ✅ | |
| Ping, real delay, speed test | ✅ | ✅ | ✅ | ✅ | ⚠️ | iOS: no ICMP; TCP ping and real delay work |
| Auto (fastest) with failover | ✅ | ✅ | ✅ | ✅ | ✅ | |
| v2rayN mode | ✅ | ✅ | ❌ | ❌ | ❌ | v2rayN is a desktop app |

## 3. Sharing

| Feature | Windows | Linux | Android | Android TV | iOS | Notes |
|---|---|---|---|---|---|---|
| Wi-Fi hotspot through the tunnel | ✅ | ⚠️ | ❌ | ❌ | ❌ | Mobile hotspots bypass the VPN (without root) |
| Local proxy (SOCKS5 / HTTP) | ✅ | ✅ | ✅ | ✅ | ⚠️ | iOS: only while connected |
| Proxy for other devices | ✅ | ✅ | ✅ | ✅ | ⚠️ | Devices on the same Wi-Fi, or on the device's own hotspot, set its address as their proxy. The mobile alternative to hotspot sharing; the app shows the Wi-Fi and hotspot addresses (and a QR code) |
| "Proxy only" mode | ✅ | ✅ | ⚠️ | ⚠️ | ❌ | Only apps with a proxy setting use it |
| System proxy | ✅ | ✅ | ⚠️ | ❌ | ⚠️ | Android 10+ / iOS: HTTP proxy through the VPN only |

## 4. Control

| Feature | Windows | Linux | Android | Android TV | iOS | Notes |
|---|---|---|---|---|---|---|
| Split tunneling by app | ✅ | ✅ | ✅ | ✅ | ❌ | iOS: managed (MDM) devices only |
| Split tunneling by domain / IP | ✅ | ✅ | ✅ | ✅ | ✅ | |
| Refuse QUIC at once | ✅ | ✅ | ✅ | ✅ | ✅ | |
| Adapter DNS settings | ✅ | ❌ | ❌ | ❌ | ❌ | Windows only |

## 5. Interface and system

| Feature | Windows | Linux | Android | Android TV | iOS | Notes |
|---|---|---|---|---|---|---|
| Tray icon | ✅ | ⚠️ | ➡️ notification | ➡️ notification | ➡️ widget / Control Center | |
| Connect at startup | ✅ | ✅ | ✅ | ✅ | ✅ | iOS: On-Demand |
| 4 languages, right-to-left | ✅ | ✅ | ✅ | ✅ | ✅ | |
| Quick toggle | ➖ | ➖ | ✅ Quick Settings tile | ❌ | ✅ Control Center, Shortcuts | |
| Scan a QR code | ➖ | ➖ | ✅ | ❌ | ✅ | |
| Send a server from the phone to the TV | ➖ | ➖ | ✅ | ✅ receives | ✅ | The TV shows a QR code / address |
| Battery optimisation exemption | ➖ | ➖ | ✅ | ⚠️ | ➖ | |
| Launcher banner, remote control | ➖ | ➖ | ❌ | ✅ | ➖ | |

## Architecture of the mobile versions

- **Shared Go code** (link and subscription parsing, server list, sing-box
  configuration, FakeIP DNS, QUIC handling, split rules, auto-fastest) is a
  library used by the desktop program and, through gomobile, by the apps.
- **Android / Android TV:** Kotlin + Jetpack Compose (Compose for TV on the
  TV). The tunnel runs on sing-box's mobile layer (libbox) inside Android's
  VpnService. Minimum Android 7 (API 24). APKs for arm64-v8a, armeabi-v7a
  and x86_64 on GitHub Releases.
- **iOS** (later): Network Extension in Swift; needs an Apple Developer
  organisation account for App Store distribution of a VPN app.

## Order of work

1. Move the shared code into a library package (desktop keeps working,
   checked by its tests).
2. Android app: connect, servers, subscriptions, tests, split tunneling,
   notification, Quick Settings tile, QR import, proxy for other devices on
   the same Wi-Fi or the phone's hotspot.
3. Android TV: remote-friendly interface, receiving servers from a phone.
4. Release pipeline: signed APKs built by GitHub Actions.
