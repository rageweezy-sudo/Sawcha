# Sawcha Next Gen Performance Engine — v9 development release

## Purpose

Sawcha is for **verified improvement to frame delivery**, not artificial benchmark scores or unrepeatable registry tweak lists. Its users include low/midrange gaming machines (AMD Ryzen/NVIDIA GeForce among others), game streamers and creators.

### Newly implemented

1. **Updated PresentMon frame analysis (multi-vendor):** newer PresentMon CSVs are parsed with optional `MsGPUBusy`, `MsCPUBusy`, and `FrameType` columns. A capture can be filtered by the game's actual EXE name; unfiltered captures containing multiple applications are rejected. Driver-generated/interpolated frame records are not silently counted as application-rendered frames. Older simple CSV exports remain supported.
2. **More useful frame statistics:** P95 and P99 frame-time, median frame-time, average application-presented FPS, 1% and 0.1% lows, frame-spike count and percentage, and optional CPU/GPU busy-time summaries. Metrics with unavailable data are not fabricated.
3. **Game-specific Insight Engine:** categorizes high GPU-busy or low GPU-busy scenes, warns about possible frame pacing and memory pressure, ranks recommended actions, explains side effects, and always includes a method to validate the change. It does not alter system settings automatically.
4. **Before/after regression detector:** compares only captures bearing the same game name and scene label, each with at least 300 usable frames. It prioritizes worse 1% lows/P99 even when average FPS increased. A single pair is explicitly *directional*, not a statistically validated performance result.
5. **GPU Compute Lab:** identifies RTX generation where possible; discusses AI tensor-compute workloads from Zagitta's research, and explicitly separates those workloads from gaming FPS. It does not copy or install Linux kernel modules, patch Windows boot code, redistribute GPU firmware, or turn off Windows security.
6. **Native UI:** added Insight Engine and GPU Compute Lab navigation and panels, while remaining Go/Win32. Navigation spacing adapts to smaller window heights.
7. **Additional automated tests:** forged cross-process frame captures, generated frame separation, optional GPU/CPU metrics, benchmark regression checks, scene-label comparisons, research-only GPU compatibility and API authentication.

### Baseline recipe (what helps gaming)

1. Choose one game, graphics preset, resolution, refresh rate, display mode, and repeatable scene.
2. Capture at least three baseline runs. Ensure the same app process is selected and game shaders are warmed up. Do not compare a loading screen with a match.
3. Make **one reversible change at a time**. Prefer in-game Reflex where supported, game's graphics quality, supported Windows graphics preferences, and supported manufacturer settings. Not every PC benefits.
4. Repeat at least three after-runs. Compare **1% lows, P99, spikes, average FPS, and power/temperature**; revert if regression is consistent.
5. During actual games, use Sawcha's low-usage mode, keep its expensive scans paused, and avoid system-wide changes in the middle of a match.

**Why no automatic driver unlock?** `Zagitta/gpu-unlock`'s own README says gaming performance is unchanged; its upstream implementation is Linux-only and focuses on FP16/BF16/TF32 AI tensor workloads. Mixing its GPL-2.0 kernel code into a Windows optimizer would also require a separate licensing and driver-safety review. The `GPU Compute Lab` is original, non-invasive detection and guidance, not a port of that module.

### Evidence and official references

- Intel [PresentMon v2.6](https://github.com/GameTechDev/PresentMon/releases) and [console CSV guide](https://github.com/GameTechDev/PresentMon/blob/main/README-ConsoleApplication.md) — application frames, GPU busy, CPU busy, frame-generation distinction; third-party executable installed separately.
- [Zagitta GPU-Unlock](https://github.com/Zagitta/gpu-unlock) — Linux compute research (not gaming acceleration).
- [NVIDIA Reflex latency guide](https://www.nvidia.com/en-us/geforce/guides/system-latency-optimization-guide/) — prefer supported game-internal low-latency features.
- [Windows 11 windowed game optimizations](https://support.microsoft.com/en-us/windows/hardware/display-graphics/optimizations-for-windowed-games-in-windows-11) — supported flip-model improvements for compatible DX10/DX11 games.

### Other-chat workstreams deliberately kept independent

- **Audio Studio:** request for configurable 18-band EQ, microphone processing, footsteps presets, and very low overhead. Separate audio branch exists, with Windows build failures reported in previous work. This release does not pretend the driver/DSP is production-ready or sign an untested kernel driver.
- **Latency Lab:** kernel-level diagnostics and developer-only VM testing, requested to stay isolated from the customer repository. Keep VM experiments in a separate private test environment; use supported Windows telemetry in public builds.
- **Windows app removal/debloat:** should be reviewable, package-scoped, opt-in, and reversible where Windows supports it. Never disable Defender, delete arbitrary services, or use hidden scripts.
- **Commercial service:** Basic/Plus ($1.99/month), owner-only signed complimentary Pro, separate Stripe backend and secure license checks. Do not place owner signing keys or customer data in public GitHub code.

### Verification / remaining gaps

Go unit tests, race checks, vet and Windows cross-compilation do **not** demonstrate an actual FPS gain on a particular PC. We cannot test Win32 graphics hardware or anti-cheat compatibility from this Linux environment. A Windows game test matrix, richer per-core/CPU-bound metrics, input-latency instrumentation, in-game overlay design review, audio driver signing, and actual payments/fraud reviews are separate milestones.
