# SAWCHA Audio Studio — setup and compatibility

## What works in this development branch

- Native Windows **AUDIO STUDIO** view with editable gains for 18 peaking EQ bands (32–12,000 Hz in 0.5 dB steps), preamp, and built-in Flat/Footsteps/Competitive FPS/Balanced/Music starting presets.
- Native **SETUP WIZARD**: select Basic/Pro preference, select the 18-band EQ module and the *planned* microphone module, install Equalizer APO externally, open its Device Selector, optionally filter a specific playback endpoint, save preferences, write 18 EQ bands to Equalizer APO, check setup status, or remove SAWCHA's changes.
- Local authenticated API validates configuration and stores audio settings in SAWCHA's existing preferences folder.
- SAWCHA writes a dedicated Equalizer APO `SAWCHA_EQ.txt` file and inserts a single managed Include block at the end of `config.txt`. Existing settings are preserved; a copy of the original `config.txt` is backed up.
- Rolling back removes only SAWCHA's Include block. Equalizer APO, user-owned filters, and your device configuration are left alone.

**Not implemented or unverified:** a standalone signed SAWCHA audio driver, automatic physical device attachment, native WASAPI audio stream capture/routing, microphone denoising that affects other Windows applications, virtual microphone/speaker devices, measured sound improvements, production-grade Pro audio features or verified Pro audio entitlements. The existing Go microphone DSP module is not attached to any Windows input. Pro is a preference in this wizard, **not** an unlock or entitlement.

## Windows 10/11 installation steps

1. Install or run SAWCHA from an official publisher build. Windows 10 and Windows 11 on an **Intel or AMD x64 computer** use the same compiled build. Audio processing runs on the audio endpoint chosen by Windows, not on CPU-brand-specific driver code.
2. Open **AUDIO STUDIO → SETUP WIZARD**.
3. Pick **BASIC / FREE** or mark **PRO / SELECT** (no Pro functionality is granted by selecting the label). Enable **18-BAND EQ**; microphone selection is retained as a future feature request.
4. Choose **DOWNLOAD** to open the authentic Equalizer APO page at https://sourceforge.net/projects/equalizerapo/ . Download its appropriate installer and complete the installer locally. SAWCHA does **not** silently install or tamper with your Windows audio drivers.
5. Use **DEVICE SELECTOR** to enable Equalizer APO for the *correct playback device* (headset / speakers). Leave input devices unchecked unless you deliberately want audio modifications on input. Accept any recommended reboot; a newly attached APO commonly requires reboot.
6. Enter your intended playback endpoint's device name in the filter box if you want additional targeting, or leave it blank to affect all endpoints enabled inside Equalizer APO. **SAVE SETUP**.
7. Return to **AUDIO STUDIO** and select one of the starting presets. Modify any of the 18 gains and preamp with +/- buttons. Choose **APPLY TO WINDOWS**. SAWCHA writes the validated Equalizer APO command file.
8. Select **CHECK STATUS** to check whether the file is referenced; then play audio, compare with a flat preset or remove SAWCHA's EQ, and ensure you hear an actual change. A detected file is **not** evidence that Windows has activated the filter on the chosen device.
9. If no change is audible: confirm Windows is outputting to the selected device, check Equalizer APO Device Selector, reboot if you attached it recently, verify audio enhancements are enabled as required for your driver, and compare using a player in shared mode. See https://sourceforge.net/p/equalizerapo/wiki/Documentation/ .
10. To revert SAWCHA changes, choose **REMOVE EQ**. This leaves other Equalizer APO files and Windows drivers intact. The original `config.txt.sawcha-original.bak` remains as an additional manual recovery copy.

### Supported and unsupported paths

- Supported target: regular Windows 10/11 shared-mode audio endpoints with Equalizer APO correctly installed and functioning. Intel/AMD CPU branding does not change the DSP math.
- Not promised: ASIO paths, exclusive-mode audio sessions, certain USB/HDMI/Bluetooth devices or vendor drivers that bypass the selected Windows APO, protected playback, or all third-party apps. Some devices disable audio enhancements, which can prevent the effect from running.
- ARM-based Windows devices are not part of the validated Intel/AMD x64 target; test their drivers and APO architecture independently.
- This branch's Windows CI validates compilation/unit tests only. Test audio quality, actual sound routing, installer, device selection, reboot, mic path, and rollback on real Windows 10 and 11 hardware before enabling this in a public release.
- Never disable Windows security, alter anti-cheat software, or inject game DLLs to try to force coverage.

## What SAWCHA changes

```text
Equalizer APO config\
    config.txt                 # Existing user file with one SAWCHA Include block appended
    SAWCHA_EQ.txt              # Owned by SAWCHA; 18 peaking bands and preamp
    config.txt.sawcha-original.bak # Pre-install snapshot (first activation only)
```

Additional Equalizer APO components remain owned by their upstream publisher. Users are responsible for confirming third-party software licenses when redistributing their installers; SAWCHA does not bundle Equalizer APO binaries.

## If you hear distortion

Lower the preamp and band gains, check volume-matched comparisons, and avoid huge narrow boosts. SAWCHA's default preset headroom is a starting point, not a proof against every possible combination of signal peaks and filters. If you need a full PCM limiter, develop and measure it in a tested real-time user-mode DSP service instead of a guessed driver.
