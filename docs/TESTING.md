# Release test checklist

Run every item on a real Windows machine and tick it off. If something fails,
keep `C:\ProgramData\v2tun\v2tun.log`.

Leak test sites: https://browserleaks.com/ip, https://browserleaks.com/webrtc,
https://dnsleaktest.com (Extended test).

Automated end-to-end tests (as Administrator, internet and at least one server
required; they change the firewall and routes and restore them):

```powershell
$env:V2TUN_E2E = '1'
go test -tags "<tags from build.ps1>" -run TestE2E -v ./cmd/v2tun
```

## 1. Basic connection

- [ ] Connects with the built-in core; the exit IP is shown and the leak test passes.
- [ ] Connects through v2rayN (with v2rayN's own TUN mode off); same results.
- [ ] Websites, YouTube and a large download work.
- [ ] Disconnect takes less than 5 seconds and normal internet comes back.
- [ ] Connect and disconnect ten times in a row: nothing gets stuck.
- [ ] "Proxy only" mode: the local SOCKS5/HTTP port works, the Windows proxy
      setting is set while connected and restored afterwards.
- [ ] Local proxy with "Allow from local network": another device can use it.

## 2. Leaks (most important)

- [ ] Kill switch on: browserleaks and dnsleaktest show only the server's IP and DNS.
- [ ] Kill switch **off** (leak guard on): still no real IPv6 and no local ISP DNS.
- [ ] WebRTC on browserleaks shows only the server's IP, or no public IP.
- [ ] Close v2rayN or the core while connected: with the kill switch on, the
      internet is fully blocked (no site opens with the real IP).

## 3. Sudden termination

- [ ] While connected, end v2tun with Task Manager → End task: normal internet,
      the original time zone and a clean hosts file
      (`C:\Windows\System32\drivers\etc\hosts`) come back.
- [ ] Same with End process tree, then start v2tun again: the log says the
      previous run was closed without disconnecting, and everything is clean.
- [ ] Hard reset of Windows while connected, then start v2tun: everything is clean.
- [ ] Same in "Proxy only" mode: the Windows proxy setting is restored.

## 4. Network and sleep

- [ ] While connected, switch from Wi-Fi to LAN (or a phone hotspot): the connection continues.
- [ ] Cut the internet for 30 seconds: the connection comes back without leaking.
- [ ] Put the laptop to sleep while connected and wake it up.

## 5. Servers

- [ ] Add a subscription, a single link, an .ovpn file and AnyConnect.
- [ ] Ping, real delay and speed test give results for every server.
- [ ] "Auto" switches to another server during a download: the download is not interrupted.
- [ ] Break the selected server (e.g. cut its internet): auto switches to a working one.

## 6. Windows

- [ ] "Start with Windows" + "Auto connect": after a reboot it connects without an Administrator prompt.
- [ ] The tray menu is correct in all four languages.
- [ ] Title bar: dragging, minimize, close (goes to the tray).
- [ ] Both Windows 10 and Windows 11.
- [ ] A non-Administrator user cannot open `C:\ProgramData\v2tun\servers.json`.

## 7. Stability

- [ ] Stays connected for 24 hours with normal use: memory in Task Manager does not keep growing.
- [ ] No `panic in` line in the log.
