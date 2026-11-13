# SAWCHA strict latency protection

**Scope:** This policy aims to keep unnecessary SAWCHA overhead low and reject speculative latency tweaks. It **cannot guarantee unchanged or lower latency on every Windows device**. CPU scheduling, drivers, displays, network routes, codecs and application paths can independently affect latency.

## Default behavior

- **STRICT LATENCY: ON** in Settings by default, including users upgrading from an older `app_preferences.json` that did not contain the setting.
- Automatic game process detection uses read-only Windows Toolhelp snapshots. It does not inject into games, hook mouse input, or change thread/interrupt priorities.
- During a recognized running game, **no recurring heavy PowerShell / CIM / WMI live telemetry is launched**, even if the regular quiet-monitor toggle was disabled. The UI uses a reduced refresh cadence. Monitoring resumes when game detection says the game closed.
- Windows registry/Game DVR/Game Mode/power-plan preset mutations are **blocked** by default. The user can explicitly turn strict protection off in Settings *after a warning*, record before/after data, and opt into those experiments. Restore is always available.
- Pointer-speed changes are similarly blocked under strict protection; mouse settings are not a proven shortcut to hardware input latency.
- Existing scans, SFC/DISM repairs, sound setup, app sign-in and read-only diagnostics are still available, but may use resources when invoked explicitly. Avoid running scans or installs during matches.

## Audio-specific decisions

- Flat EQ is a **true zero-filter configuration**: SAWCHA emits no active peaking filters, delay node, convolution or gain change.
- SAWCHA's 18-band Equalizer APO generator omits 0 dB filters; other bands use minimum-phase-style peaking IIR filters, not block-queued linear-phase FIR convolution.
- Do not add resamplers, unnecessary capture/render loopback, spatial widening, channel delays, recording, remote inference, or user-mode hop(s) solely to run playback EQ. Do not insert any `Delay:` or `Convolution:` command into SAWCHA's managed EQ file without a separate measured opt-in.
- A non-flat EQ still consumes some audio-processing CPU time and alters phase/group delay. **It is impossible to prove identical end-to-end latency without actual audio-driver/device measurements.** User installs and attaches Equalizer APO explicitly and can remove SAWCHA EQ in one action. No extra signed kernel driver is installed by SAWCHA.
- Future microphone denoising, echo cancellation and virtual devices must measure buffering, total round-trip delay, glitches and CPU/underruns. Prefer a low-latency bypass route whenever effects miss the real-time budget; restore unprocessed input on failure, never silently increase buffers.

## Compare frame pacing using PresentMon

1. Keep the same display, Windows power mode, GPU settings, game scene and in-game settings.
2. With the game running, open SAWCHA **FRAME TIME LAB** and choose **CAPTURE BASELINE**. Ensure the capture completes.
3. Change **one** setting deliberately, only if strict protection was intentionally disabled or if applying a separately consented EQ.
4. Replay the same scene and choose **CAPTURE AFTER**. For better confidence, capture multiple A/B/A/B trials instead of relying on a single run.
5. Choose **COMPARE LATENCY**. The comparison requires two distinct captures with the same game/label and 300+ frames each. It reports P99 and median frame time and 1%-low FPS.
6. If P99 or median increases more than 3%, or 1%-low FPS drops more than 5%, SAWCHA reports **possible regression** and recommends restoration. These thresholds are conservative signals, **not a formal statistical confidence test**. No automatic OS rollback is attempted because a single capture cannot prove causality and other unrelated Windows changes may have occurred.
7. For *input-to-display* latency, use an appropriate hardware latency tester or a supported software measurement method. PresentMon frame times alone do not measure input latency.

## Additional measurement needed before any release claims

| Domain | Measure before and after | Failure/revert condition |
| --- | --- | --- |
| Frame pacing | PresentMon median, P99, 1% lows, stutters on same game scene | Measurable regression, repeated trials |
| Mouse/input | Click-to-photon or verified raw input timestamp measurements | Added buffering, skipped/duplicate input or statistically meaningful worse latency |
| Speaker audio | Loopback/device round-trip latency, glitches, peak CPU on Windows shared-mode endpoints | Buffering increase or glitches; bypass EQ |
| Microphone | Capture-to-virtual-mic delay, words gated, round-trip, device change recovery | Voice cutouts or delay above chosen budget; bypass effects |
| Networking | Repeat comparable RTT, jitter and packet loss tests on same endpoint/path | Packet loss/jitter/RTT regression; restore network changes |
| Drivers/DPC | Windows Performance Recorder / Analyzer DPC/ISR traces under same workload | New spikes or underruns correlated with SAWCHA processes |
| Idle/background | SAWCHA CPU/time and disk I/O, frame-time baseline with app off/on | Avoid heavy background work; disable unnecessary periodic telemetry |

Microsoft Windows Performance Counters should not be sampled faster than once per second; high-resolution event timelines are better captured with ETW: https://learn.microsoft.com/en-us/windows/win32/perfctrs/about-performance-counters

DPC/ISR analysis with Windows Performance Analyzer: https://learn.microsoft.com/en-us/windows-hardware/test/wpt/cpu-analysis

Equalizer APO filter documentation: https://sourceforge.net/p/equalizerapo/wiki/Configuration%20reference/

## Compatibility

This policy is CPU-brand independent: both Intel and AMD x64 Windows 10 and Windows 11 systems can run the same SAWCHA binaries. It does not imply that every vendor audio driver supports Equalizer APO. WASAPI exclusive mode, ASIO, Bluetooth buffering, USB input, GPU display queues and ISP latency may be outside SAWCHA's control.

**Release gate:** no claim of a universal decrease/no-regression until hardware tests confirm the particular claim for each tested environment. Build/test CI is necessary but never substitutes for audio and input measurements on real devices.
