# SAWCHA — what we've built, in plain English

This is a factual project change log based on the requests in this conversation. “Built in source” does not mean it has been tested on every Windows PC. Statuses below separate implemented code, external setup, and work still needed.

## 1. The original optimization and repair app

- Added a Windows application for common diagnostics such as SFC, DISM, CHKDSK scans, event logs, driver lists and network information.
- Added optional gaming/creator presets using a small set of Windows Game Mode/Game DVR settings. Original settings are backed up for restoration. Those changes MAY help some systems; no universal FPS guarantee.
- Added a dark-gray background with red text and controls, later rebuilt as a native Windows Go/Win32 application so the main UI does not depend on JavaScript or a browser.

## 2. Automatic startup scan and games

- The app begins by scanning CPU, memory, GPU, OS and installed games.
- Detects installed games from supported Steam, Epic, launcher folders and manually added .exe files; discovery is not universal (some Game Pass/UWP titles are harder to detect).
- Animated startup progress screen and separate Game Library, DPI, and keybind pages.
- Launches detected games through supported launcher links/paths.

## 3. Better performance diagnosis

- Live CPU/RAM and NVIDIA telemetry where available, plus a list of processes consuming memory.
- Optional Intel PresentMon integration for frame times, FPS, 1% lows and comparative benchmarks. PresentMon must be installed separately; no built-in universal FPS overlay is shipped.
- Plain-language descriptions of each adjustment, benefits, side effects and ways to reverse changes.
- NVIDIA and AMD vendor links/settings panels rather than pretending to control unsupported drivers.

## 4. Player settings and help

- DPI and sensitivity calculator, game-specific mouse/controller/keybind guides; physical mouse sensor DPI still requires mouse vendor support.
- Offline troubleshooting guidance plus an opt-in online AI assistant using the user's own API credentials.
- Native offline guest login; Google sign-in flow requires a Google OAuth desktop client ID supplied by the publisher. No Google account automatically becomes a developer/admin account.
- Local developer diagnostic mode with a separate passphrase. This is distinct from the PRIVATE owner license issuer.

## 5. Safer distribution and memberships

- Public source repo, .gitignore, security documentation and GitHub Actions Go build/tests. Production secrets and private developer vault MUST stay outside the public source.
- Code-signing preparation and signature validation tools; actual publisher verification needs a real certificate and signed release.
- Customer Plus membership UI and a separate backend supporting Stripe Checkout, verified webhooks, encrypted subscription data, device-restricted keys and a Stripe customer billing portal (proposed $1.99/month). A complete live subscription transaction is NOT yet validated.
- Basic functionality remains available without payment.

## 6. This update: complimentary Pro, Settings, and game-aware low-usage mode

- Added a SEPARATE, Windows-only owner executable that protects an Ed25519 signing key with Windows DPAPI and issues time-limited **complimentary Pro grants** (1–365 days).
- Signed keys can be issued only by the key holder; the public server can only verify them, never mint them. The owner tool and private key are NOT included in the public repository or customer ZIP.
- Signed grants need no Stripe payment, activate on the first verified installation, cannot activate on a second installation, and can be revoked with a short-lived signed owner command. The server stores only hashed device IDs and grant IDs; its own database and secrets live on private hosting.
- Added a real Settings tab with toggles for game-process detection, in-game low-usage behavior and confirmation warnings, a selectable 60/90/120-second quiet monitoring interval, and Reset Defaults.
- Automatically checks for known running game processes every ~15 seconds using Windows read-only process snapshots. Only games found in the user's library are matched; support varies by game executable names.
- When a detected game is running, Sawcha reduces its own dashboard refreshes and expensive monitoring. Normal monitoring resumes when the game exits. It does not inject into games, kill applications, manipulate anti-cheat, silently change CPU/GPU clocks or force affinity/priority settings.
- Before deep scans, repairs, preset changes and restoring Windows settings during gaming, the app presents a warning. Its local API also refuses those changes without explicit confirmation, unless the user has intentionally turned warnings off.
- The app's per-installation identifier is DPAPI-protected on Windows instead of being transferable as plain JSON. This reduces casual copying, but no user-mode anti-piracy protection is impossible to bypass.
- Added unit/integration tests for forged/expired/revoked gift tokens, repeat/device-limited use, persisted settings, game process matching, in-game safety gates and complimentary membership activation.

## What is still NOT done or validated

- A **real Windows hardware/anti-cheat compatibility pass**, actual performance measurements across AMD/NVIDIA machines, and benchmarking frame-time improvements.
- An independent security audit, legitimate Authenticode signature and trusted installer reputation.
- A fully configured Stripe $1.99 recurring price, test checkout, webhook registration, email delivery, tax settings, public support/recovery workflow, full premium-entitlement enforcement, and production activation.
- Automatic safe overclocking and physical mouse sensor DPI; these need vendor APIs and hardware-specific testing.
- A universally compatible in-game overlay (would require separate safe capture/overlay technology and anti-cheat compatibility review).
- Guaranteed FPS gains, automatic “AI tuning” that rewrites graphics settings, or every requested optimization from other proprietary products.

**Bottom line:** This release improves legitimacy and game safety while giving the developer an actually separate, signed complimentary-license workflow. It is a development release, not a production-certified FPS booster or finished payments business.

## 7. Next Gen v9 — evidence-driven performance and GPU compute

- Used research from Intel PresentMon 2.6 to measure **CPU and GPU frame activity** when the capture file provides that information.
- Stopped mixing unrelated programs' captured frame data; separate real game frames from generated frames when the CSV identifies them.
- Added a native **Insight Engine** screen that ranks potential improvements from actual captured measurements, displays a red/gray frame-time graph, and explains the risks and how to test each change.
- Added before/after warnings for frame pacing getting worse even if average FPS rises. One pair of runs is a clue, not proof of a faster PC.
- Added **GPU Compute Lab** based on original research about NVIDIA compute workloads. It identifies RTX hardware families but does **not** copy the Linux kernel unlock, make unsupported Windows firmware patches, or claim that tensor unlocks increase game FPS.
- Extended automated tests for these features and kept the native no-JavaScript Windows UI.
- Retained separate audio, VM/kernel diagnostics, and debloat research workstreams from the other related chats; they are not falsely marked as fully implemented here.
