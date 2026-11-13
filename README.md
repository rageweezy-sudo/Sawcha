# SAWCHA // OPTIMIZER

A native Windows 10/11 desktop optimization and troubleshooting app, built in Go with a Win32 interface. This repository contains the **public customer application source**. It does **not** contain the private developer key vault, payment secrets, issued license keys, customer records, or signing credentials.

> **Status:** Development release v6.0. The code was cross-compiled for Windows x64 and passed existing Go unit tests and `go vet` in a Linux build environment. The native UI, game discovery, NVIDIA monitoring, and performance changes still need validation on real Windows hardware. No FPS gain is guaranteed.

## Features

- Startup hardware scan and installed game discovery (Steam, Epic, and selected launcher locations).
- Gray/red native UI with separate game, keybind, DPI, live-monitoring, and diagnostic views.
- Gaming/Creator optimization presets with backup and restore of supported changes.
- Frame-time measurement via an optional, separately installed Intel PresentMon console executable.
- DPI calculator and recommended game keybinds (these are **guides**, not automatic hardware DPI detection or in-game modifications).
- Offline troubleshooting suggestions; optional online assistant using a user-supplied API key.
- Google Desktop OAuth with PKCE when you configure **your own** desktop OAuth client ID. Guest access is available without it.
- Windows SFC/DISM repair helpers, network tests, and system-information summaries.

## Build the Windows app

Requires [Go 1.23+](https://go.dev/dl/) on Windows. No JavaScript, Node.js, Python, Electron, or web browser is required for the main interface.

```powershell
cd src\optimizer
go test ./...
go vet ./...
go build -ldflags="-H windowsgui" -o ..\..\SAWCHA_OPTIMIZER.exe .
```

From Linux/macOS, you can **compile** (not test the Windows UI) with:

```bash
cd src/optimizer
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags='-H windowsgui' -o ../../SAWCHA_OPTIMIZER.exe .
```

After building, use the included `packaging/` scripts if you want desktop shortcuts or signing verification. Review scripts before use. The program currently has **no embedded or hosted billing backend**, no Stripe Checkout/webhooks, and no live Plus key activation. Do not advertise paid entitlements as active.

## Safe configuration

- Google OAuth: create a **Desktop** client in your own Google Cloud project, then enter the **client ID** in-app. Never commit OAuth client secrets or downloaded credential JSON.
- Online AI: any user-supplied API key belongs only on the user's computer; never embed a key into this repository or a release binary.
- Signing: obtain a legitimate certificate through your own account, keep the certificate/private key off GitHub, and sign only final binaries. Signing does not guarantee Defender acceptance.
- Payments/licensing: must be implemented and secured as a **separate** private backend before offering Plus subscriptions. The developer vault is intentionally excluded.

See [SECURITY.md](SECURITY.md) and [docs/RELEASE_CHECKLIST.md](docs/RELEASE_CHECKLIST.md).

## Safety

No antivirus disabling, anticheat bypass, aggressive overclocking, or hidden persistence. Some system changes require Windows administrator rights; keep backups and evaluate FPS/frame-time results in the same game scene.

## License

No open-source license has been granted in this repository. Public visibility alone does not grant permission to redistribute or modify the source.
