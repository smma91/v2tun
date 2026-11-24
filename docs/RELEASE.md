# Releasing v2tun

## 1. Licence (required)

v2tun links sing-box and the sing libraries, which are **GPL-3.0-or-later**, so
v2tun as a whole is distributed under GPL-3.0 (`LICENSE`). For every release:

- Publish the complete source code of that version (e.g. a public Git tag).
- Ship `LICENSE` and `THIRD_PARTY_NOTICES.md` next to `v2tun.exe`.
- Regenerate the notices when dependencies change:
  `node tools/notices.js "<build tags from build.ps1>"`
- `wintun.dll` must be shipped unmodified (Wintun binary licence).

## 2. Version

Set it at build time: `.\build.ps1 -Version 1.0.0` (default in `cmd/v2tun/app.go`:
`0.9.0-beta`).

Easiest: push a tag. `git tag v1.0.0; git push origin v1.0.0` makes the
GitHub Actions workflow (`.github/workflows/build.yml`) run the unit tests,
build the exe with that version and create a **draft** release with
`v2tun-1.0.0-windows-amd64.zip` (exe, LICENSE, notices, README) and
`SHA256SUMS.txt`. Sign the exe (step 3) before publishing the draft if you
have a certificate.

## 3. Code signing (required to avoid antivirus / SmartScreen blocks)

v2tun changes the firewall (WFP), routes, DNS and the hosts file and embeds a
network driver loader; unsigned, Windows Defender flags it (it already flagged
an earlier build as `Trojan:Win32/Bearfoos.A!ml`).

1. Buy a code-signing certificate. EV (hardware token) gives immediate
   SmartScreen reputation; OV works but builds reputation over time.
2. Sign every release:
   `signtool sign /fd SHA256 /tr http://timestamp.digicert.com /td SHA256 /a v2tun.exe`
3. Submit the signed file to Microsoft as a false positive:
   https://www.microsoft.com/en-us/wdsi/filesubmission (Software developer).
4. Optionally check it on VirusTotal before publishing.

## 4. Checklist

- [ ] `TESTING.md` completed on Windows 10 and 11.
- [ ] `go vet` clean, unit tests pass.
- [ ] Version set, signed, notices regenerated, source tagged.
