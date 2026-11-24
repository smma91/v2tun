# Builds v2tun.exe. Uses Go 1.26 (downloaded automatically by the go command).
#   .\build.ps1                    -> version from app.go
#   .\build.ps1 -Version 1.0.0     -> release build
param([string]$Version = '')
$ErrorActionPreference = 'Stop'
$env:GOTOOLCHAIN = 'go1.26.11'
# Some antivirus products lock files in %TEMP% while Go links; use a local folder.
$env:GOTMPDIR = Join-Path $PSScriptRoot '.gotmp'
New-Item -ItemType Directory -Force $env:GOTMPDIR | Out-Null
Set-Location $PSScriptRoot
if (-not (Test-Path rsrc_windows_amd64.syso)) {
    go run tools/genicon.go
    go run github.com/akavel/rsrc@v0.10.2 -manifest app.manifest -ico app.ico -arch amd64 -o rsrc_windows_amd64.syso
}
# Wails needs desktop,production. The rest enables the sing-box protocols:
# with_quic (Hysteria, Hysteria2, TUIC), with_utls (Reality, uTLS),
# with_wireguard + with_gvisor (WireGuard), with_naive_outbound + with_purego
# (NaiveProxy through libcronet.dll, no C compiler), with_openvpn, with_openconnect.
$tags = 'desktop,production,with_quic,with_utls,with_wireguard,with_gvisor,with_naive_outbound,with_purego,with_openvpn,with_openconnect'
$ldflags = '-s -w -H windowsgui'
if ($Version) { $ldflags += " -X main.appVersion=$Version" }
go build -tags $tags -trimpath -ldflags $ldflags -o v2tun.exe .
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "built: $(Join-Path $PSScriptRoot 'v2tun.exe')"
