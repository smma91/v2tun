<h1 align="center">v2tun</h1>

<p align="center">
  <b>一键让电脑上的所有应用都走你的代理，并带真正的断网保护。</b><br>
  适用于 <b>Windows</b> 和 <b>Linux</b> 的全局隧道，可用内置核心，也可直接用你的 v2rayN。
</p>

<p align="center">
  <a href="https://github.com/smma91/v2tun/releases/latest"><img src="https://img.shields.io/github/v/release/smma91/v2tun?label=download&color=2563eb" alt="Latest release"></a>
  <img src="https://img.shields.io/badge/platform-Windows%20%7C%20Linux-0f172a" alt="Windows | Linux">
  <a href="https://github.com/smma91/v2tun/releases"><img src="https://img.shields.io/github/downloads/smma91/v2tun/total?color=16a34a" alt="Downloads"></a>
  <a href="../LICENSE"><img src="https://img.shields.io/github/license/smma91/v2tun?color=64748b" alt="License"></a>
</p>

<p align="center">
  <a href="../README.md">English</a> | <a href="README.fa.md">فارسی</a> | <a href="README.ar.md">العربية</a> | <b>中文</b>
</p>

<p align="center">
  <img src="screenshots/connect.png" width="260" alt="Connect">
  <img src="screenshots/servers.png" width="260" alt="Servers">
  <img src="screenshots/settings.png" width="260" alt="Settings">
</p>

## 为什么选择 v2tun？

| | |
|---|---|
| 🌐 **所有应用，不只是浏览器** | 游戏、聊天软件、更新程序、终端都会走隧道，无需逐个设置代理。 |
| 🛡️ **零泄漏** | 断网保护、IPv6 与 DNS 泄漏防护、远程 DNS：泄漏测试只会显示你的服务器。 |
| 📶 **通过 Wi-Fi 共享** | 把电脑变成热点，连接上来的手机和笔记本也会走隧道。 |
| ⚡ **支持所有主流协议** | VLESS Reality / XHTTP、VMess、Trojan、Shadowsocks、Hysteria2、TUIC、WireGuard、NaiveProxy、OpenVPN、AnyConnect 等。 |
| 🚀 **始终用最快的服务器** | Ping、真实延迟、测速，以及在不中断下载的情况下自动切换服务器的“自动”模式。 |
| 🔌 **兼容 v2rayN** | 已经在用 v2rayN？v2tun 会自动找到它，并让整个系统走它的代理。 |
| 📱 **手机和电视也能用** | Android 应用(手机、平板和 Android TV),同样的服务器,支持二维码导入和分享。 |
| 🧩 **简单免费** | 一个小巧的程序，简洁的深色界面，支持 4 种语言，开源（GPL-3.0）。 |

## 功能

### 🛡️ 隐私与保护
- **全局 TUN 隧道**，覆盖所有应用（Windows 用 Wintun，Linux 用 `/dev/net/tun`）。
- **断网保护：** 隧道或代理断开时网络保持阻断，不会回落到真实连接。
- **IPv6 与 DNS 泄漏防护**，即使关闭断网保护也有效。
- **远程 DNS（FakeIP）：** DNS 查询不会离开电脑，由服务器在出口国家解析。
- **内置泄漏测试**（IP 与 WebRTC），并显示出口 IP、国家和运营商。
- **时区同步**（可选）：系统时区跟随服务器所在国家。
- **异常退出保护：** 守护进程会恢复网络、时区和代理设置。

### ⚡ 服务器与协议
- **内置 [sing-box](https://github.com/SagerNet/sing-box) 核心：** VLESS（Reality，XHTTP 通过 Xray）、VMess、Trojan、Shadowsocks、Hysteria / Hysteria2、TUIC、WireGuard、NaiveProxy、SOCKS、HTTP、OpenVPN 以及 AnyConnect / OpenConnect / Fortinet / GlobalProtect。
- **订阅**：支持 base64、sing-box 和 Clash 格式，自动更新。
- **Ping、真实延迟和测速**，单个或全部服务器。
- **自动（最快）** 故障切换，不中断已有连接。
- **v2rayN 模式：** 直接使用你在 v2rayN 中的服务器。

### 📶 共享
- **Wi-Fi 热点：** 连接的手机、平板和笔记本以同样的保护走隧道；隧道断开时它们无法上网，不会泄漏。
- **本地代理：** SOCKS5 和 HTTP 共用一个端口，可选允许局域网访问。
- **“仅代理”模式：** 不建隧道，仅将本地代理设为系统代理。

### 🧭 控制
- **分流**：按应用和域名 / IP 段，支持“排除”和“仅包含”模式。
- **智能 QUIC 处理：** 服务器不支持 UDP 时，应用会立即改用 TCP，不会卡住。

### 📱 Android 与 Android TV
- 同样的协议和订阅;可从剪贴板、**扫描二维码**(相机或图片)或把链接分享给 v2tun 来添加服务器。
- 以二维码或链接**分享任意服务器或订阅**。
- 实时速度、出口 IP 及国家、服务器 Ping、快捷设置图块、开机自动连接。
- **本地代理**,供同一 Wi-Fi 或手机热点上的其他设备使用。
- Android TV:专为遥控器(方向键)设计,主屏幕有电视横幅。
- 省电:只在应用显示时统计实时数据,Doze 休眠时内核暂停。
- Android 7.0 或更高版本。

### 🖥️ 桌面
- 托盘图标、登录时启动（无密码或 UAC 提示）、自动连接。
- 界面支持 **英语、波斯语、阿拉伯语和中文**。
- Windows 10/11 与 Linux（Ubuntu、Debian、Mint、Fedora、AppImage）。

## 下载

从 **[Releases](https://github.com/smma91/v2tun/releases/latest)** 下载最新版本。各系统的安装方法见下文。

## Windows 系统要求

- Windows 10 或 11，64 位（x64）。
- 管理员权限（用于隧道网卡、路由和防火墙）。
- [WebView2 运行时](https://developer.microsoft.com/microsoft-edge/webview2/)（Windows 11 和已更新的 Windows 10 自带）。

## 在 Windows 上安装

1. 从 [Releases](https://github.com/smma91/v2tun/releases) 下载 `v2tun-<version>-windows-amd64.zip`，并用 `SHA256SUMS.txt` 校验。
2. 解压到任意位置，运行 `v2tun.exe`（会请求管理员权限）。
3. 在 **服务器** 页添加服务器（粘贴链接或订阅），或在 v2rayN 运行时选择 **v2rayN** 作为代理来源。
4. 点击电源按钮。

设置、服务器和日志保存在 `C:\ProgramData\v2tun`（仅管理员可读）。

> **杀毒软件提示。** v2tun 会修改防火墙、路由、DNS 和 hosts 文件，并加载网络驱动。此类工具的未签名版本有时会被误报。你随时可以用本仓库的源码自行编译。

## Linux

**系统要求：** 64 位（x86_64）且带 WebKitGTK 4.1 的发行版，例如 Ubuntu 22.04+、Debian 12、Linux Mint 21 或 Fedora 37+。

从 [Releases](https://github.com/smma91/v2tun/releases) 下载对应文件：

| 安装包 | 安装方式 |
|---|---|
| `.deb` (Ubuntu, Debian, Mint) | `sudo apt install ./v2tun_<version>_amd64.deb` |
| `.rpm` (Fedora) | `sudo dnf install ./v2tun-<version>-1.x86_64.rpm` |
| `.AppImage` | `chmod +x v2tun-*.AppImage` |
| `.tar.gz` | `sudo ./install.sh` |

然后从应用菜单启动 **v2tun**。

- v2tun 不以 root 运行：窗口以当前用户运行，程序只获得两项必要的网络权限（`cap_net_admin`、`cap_net_raw`）。安装包和 `install.sh` 会自动设置；若缺少权限（例如 AppImage），界面会显示 **授予权限** 按钮，只需输入一次密码，AppImage 会同时把自己安装到 `/opt/v2tun`。
- 托盘图标需要桌面支持托盘（KDE、Xfce、Cinnamon、Ubuntu 均支持；纯 GNOME 需安装 AppIndicator 扩展）。没有托盘时，关闭窗口会将其最小化。
- 设置和服务器保存在 `~/.config/v2tun`。

## Android 与 Android TV

**系统要求:** Android 7.0 或更高版本(手机、平板、Android TV)。

从 [Releases](https://github.com/smma91/v2tun/releases) 下载 APK,并用 `SHA256SUMS.txt` 校验:

| 文件 | 适用于 |
|---|---|
| `v2tun-<version>-android-arm64-v8a.apk` | 几乎所有手机和较新的电视 |
| `v2tun-<version>-android-armeabi-v7a.apk` | 较旧的手机和大多数电视盒子 / 电视棒 |
| `v2tun-<version>-android-universal.apk` | 任何设备(体积更大) |
| `v2tun-<version>-android-x86_64.apk` | 模拟器和 x86 设备 |

打开文件即可安装(如 Android 询问,请允许从此来源安装)。在电视上,可用 U 盘或 *Send files to TV* 等应用传输 APK,或用 `adb install` 安装。

首次连接时 Android 会请求允许 VPN。如需断网保护(Kill switch),在 v2tun 中打开 **设置 → Kill switch**(会跳转到 Android 的 VPN 设置),开启*始终开启的 VPN* 和*屏蔽未使用 VPN 的连接*。

## 从源码构建

需要 Go 1.26（`go` 命令会自动下载对应工具链）和 PowerShell，无需 C 编译器。

```powershell
git clone https://github.com/smma91/v2tun.git
cd v2tun
.\build.ps1                 # 或：.\build.ps1 -Version 1.0.0
```

生成单个 `v2tun.exe`，`wintun.dll` 和 `libcronet.dll` 已内嵌其中。

**Linux：**


```sh
sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
./build.sh
sudo setcap cap_net_admin,cap_net_raw+ep ./v2tun
```

生成 `v2tun` 和 `libcronet.so`，两者需放在同一目录。测试与翻译说明见 [英文 README](../README.md)。

## 反馈

发现问题或有建议？请 [提交 Issue](https://github.com/smma91/v2tun/issues/new/choose)。粘贴日志前请删除服务器地址、密码和订阅链接。目前不接受 Pull Request。

## ☕ 支持本项目

v2tun 免费且开源。如果它对你有帮助，欢迎请我喝杯咖啡：

| 网络 | 地址 |
|---|---|
| Ethereum、BNB Smart Chain、Polygon、Arbitrum 等 EVM 网络（USDT、USDC、ETH、BNB 等） | `0x1404A760e5A30f9DD1D3AC68726673f94DC2f649` |

转账前请再次核对地址和网络。点个 Star ⭐ 或反馈问题也是很大的支持。

## 许可证

由于包含 sing-box 及其他 GPL 授权的库，v2tun 以 [GNU General Public License v3.0](../LICENSE) 发布。第三方组件及其许可证见 [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md)。

本软件按“原样”提供，不附带任何担保。请遵守你所在国家的法律以及代理服务商的条款。
