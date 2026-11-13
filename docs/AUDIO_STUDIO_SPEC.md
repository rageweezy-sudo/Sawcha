# SAWCHA Audio Studio: implementation design

**Status (2026-11-13):** This branch now includes offline Go DSP primitives, a native 18-band EQ editor with +/- controls, a selectable Basic/Pro audio setup wizard, and an Equalizer APO backend that generates and applies a managed `SAWCHA_EQ.txt` filter file after users install/attach Equalizer APO. Windows CI checks compile and unit tests. **No device-audible test, signed SAWCHA driver, real microphone routing, virtual endpoints, denoising service, or production Pro audio entitlement has been verified.** Do not advertise standalone audio features as shipping until real Windows hardware tests pass.

## User goal

A native SAWCHA Audio Studio page for competitive gaming and voice chat, with individually editable **18-band EQ**, saveable per-game presets, clear microphone speech, and automatic recovery from supported device-change events.

## 18-band EQ

Bands (Hz): `32, 45, 63, 90, 125, 180, 250, 350, 500, 700, 1000, 1400, 2000, 2800, 4000, 5600, 8000, 12000`.

- Per-band gain -12..+12 dB; preamp, preview graph, bypass, reset, save/import/export JSON.
- Built-ins: Flat, Footsteps, Competitive FPS, Balanced, Music; these are **unvalidated starting curves**, not universal improvements or specific game calibration.
- Provide clipping and loudness warnings, conservative preamp/headroom, gain smoothing and final limiting in the production audio pipeline.
- Preserve two-channel spatial information. No artificial stereo widening in competitive presets by default.
- Per-output-device and per-game profile selection. Store settings locally; no cloud audio upload.
- Provide headset tuning so curves can account for differences between headphones. Comparison should be level-matched; louder is not necessarily clearer.

## Microphone studio

- Selected physical microphone -> high-pass (plosive/rumble control) -> optional echo cancellation (with render reference) -> noise suppression -> *gentle downward expander* (not aggressive hard gate) -> compressor -> slow auto-gain (if enabled) -> de-esser (optional) -> protective limiter -> selected virtual microphone.
- Presets: Natural, Broadcast, Gaming/Discord, Noisy Room, Manual.
- User controls: input device, peak/RMS meter, auto-level target, compressor threshold/ratio, expander depth/threshold/hold/release, noise suppression intensity, echo cancellation, mic monitor with feedback warning, 10-second private test playback.
- Device-reconnect and default-device-change handling must reinitialize streams and display errors without silently switching to a remote microphone.
- A soft expander reduces cut-offs caused by overly aggressive gates, but **software cannot guarantee correction of USB disconnects, damaged cables, Bluetooth interruptions, hardware overload, network packet loss, or third-party app gating**. Show targeted troubleshooting.
- Do not enable two competing noise suppressors/AGCs without warning; provide a dry/wet comparison and a single-click disable of SAWCHA processing.

## Architecture

1. Keep the Go/Win32 app as the settings UI and control plane. Add an Audio Studio menu/page and authenticated localhost API endpoints for device enumeration, profiles, live meters, and status. Validate input ranges server-side.
2. Use a separately packaged, **user-mode Windows WASAPI audio service** for real-time audio streams and processing. No runtime allocations, blocking calls, unbounded locks, or file I/O in real-time callbacks. Configure high-priority audio threads appropriately and measure glitch/dropout rates.
3. Prefer a supported Windows **Audio Processing Object (APO)** when appropriate for effects on an existing endpoint, subject to driver registration and OEM/third-party deployment constraints. Do not assume Windows permits arbitrary app-level insertion into all physical devices.
4. If selectable `SAWCHA Virtual Microphone` and `SAWCHA Virtual Speakers` endpoints are required, build a separate SysVAD-based driver, package through INF, and have it signed through a legitimate Windows driver signing workflow. Keep it entirely optional.
5. The signed driver (if any) must have a minimal kernel footprint; process EQ, denoising, compression, AEC, profile logic and UI in **user mode**. No game DLL injection, privileged anticheat bypass, or hidden audio recording.

## Development order and acceptance criteria

- **Phase A: DSP foundation (implemented, unit-tested):** filter bank, preset model and mic expander/compressor/ceiling primitives. Pure Go, no new dependencies.
- **Phase B: Native EQ editor / setup wizard (implemented; Windows hardware UI test pending):** save audio preferences, choose Basic/Pro setup labels, enable EQ module, modify each of 18 bands in half-dB increments, choose a starter preset and apply or remove the managed Equalizer APO Include. Audio Pro selection is not an entitlement; microphone selection does not install processing.
- **Phase C: Equalizer APO real-device verification (pending):** on Windows 10 and Windows 11 x64 Intel/AMD machines, install Equalizer APO from the publisher, attach a shared-mode playback endpoint, reboot, apply SAWCHA EQ, confirm an audible change at a safe volume, verify no driver breakage and test rollback. The Windows CI environment alone does not establish success.
- **Phase C2: Optional standalone WASAPI service (pending):** select capture/render endpoints and verify real-time streams where necessary, including microphone processing, virtual endpoint routing and device changes. Test 44.1/48 kHz on multiple headsets.
- **Phase D: Production microphone processing:** integrate and verify RNNoise (noise suppression) and/or WebRTC Audio Processing (AEC, AGC), after reviewing licenses, ABI, CPU/latency budgets and build dependencies. Add hold/hysteresis expander, loudness target, smooth limiter, mic monitor and device continuity testing.
- **Phase E: Optional virtual devices:** separately test signed virtual driver on clean Windows 10/11 virtual machines with Memory Integrity and real Windows hardware. Add secure install/uninstall/rollback. Require explicit user consent and administrator permission at installation.
- **Phase F: Game-specific auto-switch:** game process detection outside the game process; compare footprints, sound localization and frame times against bypassed audio, including while SAWCHA enters low-usage game mode.

Before release, require tests for: incorrect settings, very loud audio, silent input, 1/2/6/8 channel audio, hot-plug/unplug, rapid preset switching, output format changes, sleep/resume, device permission denial, DAW/exclusive-mode conflicts, clipping, crashes and uninstall/rollback.

## Upstream references

- Microsoft SysVAD sample: https://github.com/microsoft/Windows-driver-samples/tree/main/audio/sysvad
- Microsoft APO architecture: https://learn.microsoft.com/en-us/windows-hardware/drivers/audio/audio-processing-object-architecture
- Microsoft WASAPI loopback: https://learn.microsoft.com/en-us/windows/win32/coreaudio/loopback-recording
- Xiph RNNoise (review COPYING when bundling): https://github.com/xiph/rnnoise
- WebRTC audio processing: https://webrtc.googlesource.com/src/+/refs/heads/main/modules/audio_processing/
