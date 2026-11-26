# Builds android/app/libs/libv2tun.aar: the v2tun mobile package and sing-box's
# libbox, for the Android app (gomobile). Needs Go 1.26, the Android SDK + NDK
# (ANDROID_HOME, or D:\Android\Sdk) and JDK 17+.
#   .\build\android\build-aar.ps1            all ABIs (release)
#   .\build\android\build-aar.ps1 -Fast      arm64 only (testing on a phone)
param([switch]$Fast)
$ErrorActionPreference = 'Stop'
$repo = Resolve-Path "$PSScriptRoot\..\.."
Set-Location $repo
$env:GOTOOLCHAIN = 'go1.26.11'
if (-not $env:ANDROID_HOME) { $env:ANDROID_HOME = 'D:\Android\Sdk' }
if (-not $env:ANDROID_NDK_HOME) {
  $env:ANDROID_NDK_HOME = (Get-ChildItem "$env:ANDROID_HOME\ndk" -Directory | Sort-Object Name | Select-Object -Last 1).FullName
}
# Large build cache: keep it off a full system drive if D: exists.
if (Test-Path 'D:\') { $env:GOCACHE = 'D:\Android\go-cache'; $env:GOTMPDIR = 'D:\Android\tmp' }
New-Item -ItemType Directory -Force $env:GOTMPDIR, 'android\app\libs' -EA 0 | Out-Null
$gobin = Join-Path (go env GOPATH) 'bin'
$env:PATH = "$gobin;$env:PATH"
if (-not (Get-Command gomobile -EA 0)) {
  go install github.com/sagernet/gomobile/cmd/gomobile@v0.1.13
  go install github.com/sagernet/gomobile/cmd/gobind@v0.1.13
}
$targets = if ($Fast) { 'android/arm64' } else { 'android/arm64,android/arm,android/amd64' }
# Same protocols as the desktop, except NaiveProxy (needs a native library).
# badlinkname / -checklinkname=0: libbox reaches into Go internals (as sing-box's
# own Android build does).
$tags = 'with_gvisor,with_quic,with_wireguard,with_utls,with_openvpn,with_openconnect,badlinkname,tfogo_checklinkname0'
gomobile bind -v -target $targets -androidapi 21 -javapkg io.github.smma91.v2tun -libname box `
  -tags $tags -trimpath -ldflags '-s -w -buildid= -checklinkname=0' `
  -o android\app\libs\libv2tun.aar ./mobile github.com/sagernet/sing-box/experimental/libbox
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Get-Item android\app\libs\libv2tun.aar | Select-Object Name, @{n = 'MB'; e = { [math]::Round($_.Length / 1MB, 1) } }
