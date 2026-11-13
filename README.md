# SAWCHA // OPTIMIZER

A native Windows 10/11 desktop optimization and troubleshooting app, built in Go with a Win32 interface. This repository contains the **public customer application source**. It does **not** contain the private developer key vault, payment secrets, issued license keys, customer records, or signing credentials.

> **Status:** Development release v8.0. The code was cross-compiled for Windows x64 and passed existing Go unit tests and `go vet` in a Linux build environment. The native UI, game discovery, NVIDIA monitoring, and performance changes still need validation on real Windows hardware. No FPS gain is guaranteed.

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

After building, use the included `packaging/` scripts if you want desktop shortcuts or signing verification. Review scripts before use. The program has a separate Plus API source, but the end-to-end Stripe billing system is still not configured or live-tested. The customer EXE is unsigned; do not advertise paid entitlements as active until tested.

## Safe configuration

- Google OAuth: create a **Desktop** client in your own Google Cloud project, then enter the **client ID** in-app. Never commit OAuth client secrets or downloaded credential JSON.
- Online AI: any user-supplied API key belongs only on the user's computer; never embed a key into this repository or a release binary.
- Signing: obtain a legitimate certificate through your own account, keep the certificate/private key off GitHub, and sign only final binaries. Signing does not guarantee Defender acceptance.
- Payments/licensing: the public customer-only backend is separate from the developer-only signing tool. Real payment keys, identity secrets and customer data remain private.

See [SECURITY.md](SECURITY.md) and [docs/RELEASE_CHECKLIST.md](docs/RELEASE_CHECKLIST.md).

## Safety

No antivirus disabling, anticheat bypass, aggressive overclocking, or hidden persistence. Some system changes require Windows administrator rights; keep backups and evaluate FPS/frame-time results in the same game scene.

## License

No open-source license has been granted in this repository. Public visibility alone does not grant permission to redistribute or modify the source.

## Plus membership / billing integration

The customer app now includes a **Plus Membership** screen. The separate Go backend at [`services/plus-api/`](services/plus-api/) supports Stripe-hosted Checkout, signed subscription webhooks, encrypted license storage, device-bound verification, and a customer billing portal. Backend code is public **but has no developer/admin endpoints or embedded secrets**. The original developer key vault remains separate and private.

The proposed Plus plan is **$1.99 USD/month**; Basic remains free. Configure the hosted HTTPS service and **test-mode** Stripe credentials as described in [`docs/PLUS_BILLING.md`](docs/PLUS_BILLING.md) and [`services/plus-api/README.md`](services/plus-api/README.md). Never place Stripe keys or license records in the public repository.

**Status:** The integration has local unit/integration tests using a mocked Stripe API and can cross-compile to a Windows executable. A live payment setup, hosted webhooks, persistent production database, full Windows UI test, and independent security audit remain necessary before charging customers. The app does not yet enforce every planned premium feature gate.

## Complimentary Pro giveaways and game safety (v8)

SAWCHA now accepts developer-signed complimentary Pro keys through the same hosted verification API, without Stripe payment. The owner signs each gift in a **separate private Windows tool**, not a key-generation page embedded in the customer app. The signed gift is bound to the first registered installation, expires after the chosen number of days, and can be revoked through a separately signed owner command. The private owner signing key (Windows DPAPI-protected) and its executable **do not belong in this public repository**. The server requires `SAWCHA_GIFT_PUBLIC_KEY`, which contains only a public key. See [`docs/COMPLIMENTARY_PRO.md`](docs/COMPLIMENTARY_PRO.md).

The Settings tab supports automatic running-game detection, quiet in-game monitoring, a 60/90/120-second quiet polling interval, confirmation warnings for system changes and restoring defaults. A running game never triggers automatic registry writes or process termination. Monitoring returns to normal when the game closes. Windows installation identity is protected using DPAPI, not stored as transferable plaintext.

See [`docs/PROJECT_HISTORY_PLAIN_ENGLISH.md`](docs/PROJECT_HISTORY_PLAIN_ENGLISH.md) for a plain-English feature and limitation list and [`docs/OPTIMIZER_RESEARCH.md`](docs/OPTIMIZER_RESEARCH.md) for researched design inspiration.

## Audio Studio / system EQ (development branch)

The **Audio Studio** native page now has an editable 18-band parametric EQ, preamp and Flat/Footsteps/Competitive FPS/Balanced/Music starter profiles. The **Audio Setup Wizard** lets users select a Basic/Pro setup preference and intended components, then guides them to install the separately maintained [Equalizer APO](https://sourceforge.net/projects/equalizerapo/) audio engine, select their Windows playback endpoint, and apply a managed EQ configuration with rollback. Presets remain local unless the user explicitly chooses **APPLY TO WINDOWS**.

This is intended for Intel/AMD x64 Windows 10/11 shared-mode audio, not every possible audio API or driver. The Equalizer APO installer and endpoint configuration are user-controlled. The wizard does **not** silently install third-party drivers, and **Pro selection is not a verified paid entitlement**. A virtual SAWCHA driver and real microphone routing / AI denoising are not yet implemented. CI passing is not equivalent to listening tests on actual Windows devices.

See [docs/AUDIO_STUDIO_SETUP.md](docs/AUDIO_STUDIO_SETUP.md) for setup, troubleshooting and rollback; [docs/AUDIO_STUDIO_SPEC.md](docs/AUDIO_STUDIO_SPEC.md) for technical design. The pull-request Windows CI uploads an **unsigned development binary**, not a signed consumer release.

## Strict latency protection (default ON)

SAWCHA's default is **leave unproven Windows changes alone**. Strict Latency Protection blocks speculative registry, Game Mode/Game DVR, power-plan and pointer-speed modifications. When a recognized game is running, SAWCHA pauses recurring heavyweight PowerShell/WMI live monitoring. For optional comparisons, **Frame Time Lab** captures before/after PresentMon data and warns on possible P99/median/1%-low regression; a single test is **not proof** of improved input-to-display, network, or audio latency. Users may explicitly opt out of strict protection to experiment, with existing restore options.

Audio Studio now avoids generating no-op filters for the Flat preset and skips 0 dB filters on other EQ curves. Non-flat EQ can still use CPU and affect signal phase/group delay; actual endpoint processing and total latency need real Windows hardware measurements. See [docs/LATENCY_SAFETY.md](docs/LATENCY_SAFETY.md) for the exact policy and testing requirements.

