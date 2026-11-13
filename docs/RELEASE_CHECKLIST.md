# Sawcha release checklist

This repository includes the public customer app only. **Never** push the private developer license vault, issued keys, payment credentials, signing private keys, developer passwords or customer data.

1. Run `go test ./...` and `go vet ./...` from `src/optimizer`.
2. Build the unsigned Windows x64 executable from reviewed source.
3. Test the actual native UI, first-run scan, game discovery, guest login, Google OAuth, diagnostics, FPS capture, backups and restoration on Windows 10 and 11.
4. Test with an AMD CPU and NVIDIA GPU, including machines without NVIDIA command-line utilities.
5. Check that each system optimization is reversible and improves measured performance in representative runs.
6. Inspect Defender/SmartScreen warnings and investigate any real detections. Do not advise security exclusions.
7. Sign the final unmodified executable using your verified certificate, ideally through a secured signing service, then verify its signature and SHA-256 checksum.
8. Set up and test a **separate** hosted licensing server, Stripe Checkout and signed webhooks before advertising Plus. Use test mode first and never embed Stripe secret keys into the customer app.
9. Document exactly what is and is not working; do not claim CI proved Windows hardware compatibility.

## Latency and audio release gate — mandatory

- [ ] Confirm STRICT LATENCY ON as the default after fresh setup **and** upgrade from an older preferences file. With strict ON, Windows presets and pointer speed changes must be blocked before making any changes.
- [ ] While a recognized game is running, confirm SAWCHA does not execute recurring PowerShell/CIM/WMI live-monitor work; compare FPS/frame pacing with SAWCHA running and fully closed.
- [ ] Collect repeat before/after PresentMon captures under the *same game scene*. Check median/P99/1%-low and investigate any >3% frame-time change or >5% fall in 1%-low. Treat changes within threshold as **unproven**, not identical performance.
- [ ] For mouse input, measure hardware-to-photon latency rather than equating FPS or pointer-speed settings with latency.
- [ ] For audio output, verify Equalizer APO is attached to the actual device and test it on Windows 10 and 11 Intel/AMD x64, with USB, onboard and Bluetooth playback as supported. Compare loopback latency and audio underruns against bypass.
- [ ] Verify Flat EQ generates no active filters or delay. Verify non-flat EQ adds no explicit `Delay:` or convolution stage and safely removes only SAWCHA's own directives on rollback.
- [ ] If microphone routing is added, measure capture-to-output delay, speech gating and glitches, and verify bypass when unavailable or unhealthy.
- [ ] Collect Windows Performance Recorder/Analyzer DPC/ISR traces if glitches occur. Do not claim all drivers or applications have been tested.
- [ ] Publish a signed release **only after** independent on-device latency tests and rollback tests. CI compilation/unit tests alone are not performance or compatibility certification.

Details: [LATENCY_SAFETY.md](LATENCY_SAFETY.md).
