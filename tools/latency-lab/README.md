# SAWCHA Latency Lab — private diagnostic test companion

**Development app; not a kernel driver or official SAWCHA optimizer release.**

A separate Windows 10/11 x64 test application to collect detailed read-only performance diagnostics, conduct a short opt-in Microsoft WPR/ETW **kernel-provider trace**, export a curated JSON report, and analyze it locally or through an **optional OpenAI API chat interface**.

## Features

- **Standard scan** — on demand, records Windows version/build, boot time, CPU load and clocks, CPU performance counters including DPC/interrupt summary, GPU driver and display mode, RAM modules and utilization, disk models/queue/latency counters, audio devices, USB controllers, network adapter link speed, BIOS/board, power plan and top process resource usage, including SAWCHA-specific process snapshots when present.
- **Deep scan** — additionally collects capped signed-driver inventory for relevant device classes, recent system event IDs (not event contents) and grouped Windows service counts. A warning reports unavailable fields. PowerShell/CIM might require additional permissions or fail if WMI is broken; data collection is bounded by a timeout.
- **Kernel trace** — a separate **12-second WPR GeneralProfile ETW capture**, initiated ONLY after an explicit privacy and performance warning. WPR comes with Windows; elevation may be required. Saves an **ETL** file to `%LOCALAPPDATA%\Sawcha\LatencyLab\traces`. Analyze it using Microsoft Windows Performance Analyzer. ETL may contain personal paths/process info, **is not uploaded**, and is not included in JSON reports.
- **AI chat** — entering a personal OpenAI API key and clicking **ASK OPENAI** calls the OpenAI Responses API using the `gpt-6-luna` model. The user sees an opt-in checkbox to include the sanitized report. Default: **question only**, no report. The key is not saved to disk, logged or embedded in the public executable. A ChatGPT Plus plan is distinct from API access/billing. This is a new API conversation, not the same ChatGPT chat.
- **Offline analysis** — basic rules/recommendations, no API key or outbound request required.
- **Export** — download or copy the local JSON and submit it manually to ChatGPT for further review.

## Actual optional Windows app removal (explicit user-controlled)

**This build CAN uninstall supported optional AppX/Store apps for the current Windows user.** Unlike a pure inspector, the feature now uses Microsoft `Remove-AppxPackage` with a confirmation workflow, per-app results and post-removal verification. Microsoft warns that app removal can be irreversible and can erase local app data.

1. Click **FIND INSTALLED OPTIONAL APPS** and expand the **Optional apps** cards.
2. Tick up to six installed apps to remove. No apps are preselected.
3. Click **PREVIEW SELECTED REMOVALS**. Read every app-specific warning (especially **Xbox Game Bar**, **Phone Link**, **Outlook**, **Media Player** and **Clipchamp**).
4. Check the data-loss warning, type `REMOVE SELECTED APPS` exactly, then click **UNINSTALL SELECTED APPS FROM THIS WINDOWS USER**. A final Windows-app confirmation appears in the browser.
5. Windows re-queries the exact allowlisted package names, blocks NonRemovable/framework/resource/bundle packages and uninstalls using its supported `Remove-AppxPackage` command. It verifies current-user package registration and displays removed / not_installed / protected / failed for each item.
6. **SAVE REMOVAL RECEIPT** to keep a record. SAWCHA rescans after removal. Do not run cleanup during a game.

Eligible apps include Copilot, Solitaire, Clipchamp, Feedback Hub, News, Get Help, Phone Link, Xbox Game Bar, Weather, Maps, People, Media Player/Groove, Movies & TV, Tips, legacy Cortana and the new Outlook. Support is conditional on which versions Windows actually has installed, and some packages are system-protected. This does **not** uninstall arbitrary desktop EXEs or traditional Win32 programs.

**Hard boundaries:** never uninstalls Microsoft Store, Windows Security, Windows shell, Edge/WebView2, Gaming Services, WinGet, drivers or core services. No `-AllUsers`, no provisioned-image removal, no registry editing, no security bypass, no auto-run, no remote execution. This is an original SAWCHA implementation; FluentTweaker upstream remains unmodified and unbundled. The [official FluentTweaker releases](https://github.com/builtbybel/FluentTweaker/releases) are linked for comparison only. No universal latency benefit is claimed.

**Rollback:** AppX uninstallation is not automatically reversible. Reinstall eligible apps via Microsoft Store if offered, and make sure your account data is synchronized before removal. No state restore point is guaranteed.

## Run and build

Run the unsigned test build EXE on Windows 10/11 x64. The tool starts a browser UI on **127.0.0.1** using a random OS-selected port. It does not open a firewall port or install anything. Close the console window to exit. The only outbound request is a deliberate **ASK OPENAI** action using the personal API key. Windows AppX uninstall is a local operation requiring explicit user action.

To build from source with Go 1.23+:

```powershell
cd tools\latency-lab
go test ./...
go vet ./...
go build -o SAWCHA_Latency_Lab.exe .
.\SAWCHA_Latency_Lab.exe
```

For WPR ETW tracing, launch the tool from an **administrator** PowerShell session if the normal run reports insufficient permissions. Running with admin is not necessary for standard diagnostics. If a trace is interrupted unexpectedly, use an elevated `wpr -cancel` (only after confirming no other desired WPR recording is in progress).

## Security and privacy limits

- No kernel memory reading, unsigned driver installation, test signing, arbitrary hardware I/O, disabling Windows Defender / Memory Integrity / driver signing / anticheat, game injection or process manipulation.
- Diagnostic JSON excludes IP/MAC, serial, hostname, username, full process command lines, event messages and file contents by design. Device names and process names can still reveal software or hardware; **inspect reports before sharing**.
- ETL traces can contain more sensitive information than the JSON. Treat them as private and do not upload to AI without personally reviewing/sanitizing them.
- No background monitoring, scheduler, telemetry, autorun, cloud upload or registry tweaks. The separate selected-app uninstaller performs explicitly confirmed current-user AppX removals. The app's scan/WPR actions can temporarily affect performance. **Do not perform trace capture during competitive matches**.
- All API routes are localhost-only, require a per-launch random session token and reject foreign Origin/Host. The web UI loads no external libraries. API credentials stay in the browser input field for that session and go only through a localhost POST to a fixed OpenAI HTTPS destination on explicit request.
- WPR is a Windows Microsoft-signed trace controller for *kernel ETW providers*. It is kernel-originated telemetry, not unrestricted kernel driver execution. A future custom driver would require a separate threat model, code signing and Windows HLK testing.

## Missing measurements and planned integration

This test build **cannot currently** measure physical click-to-photon latency, precise audio round-trip delay, packet-level jitter, vendor-specific GPU hardware sensor readings, or automatically **parse ETL** traces into per-driver DPC tables. Use WPR/WPA (and specialized latency tools) and compare against controls. Read-only CIM CPU `%DPC` summaries are **not** substitutes for WPA per-driver DPC/ISR trace analysis.

A kernel telemetry result is not proof of a safe or successful latency optimization. To evaluate SAWCHA, capture the same game scene and audio setup with optimizer **off/on**; retain baseline and restore changes on regression. This companion is intentionally separate from the production optimizer.
