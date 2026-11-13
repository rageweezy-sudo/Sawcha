SAWCHA // OPTIMIZER 6.0 — NATIVE WINDOWS EDITION
Built by sawcha
==============================================

WHAT THIS IS
A real Windows 10/11 desktop program with a native Win32 interface written
in Go. No JavaScript, Electron, WebView, Chrome, Python, Node.js, or external
runtime is required to run the app. Google sign-in intentionally uses your
normal browser for the official Google authorization screen.

INSTALL (64-BIT WINDOWS 10 OR 11)
1. Extract the entire ZIP archive.
2. Double-click SAWCHA_OPTIMIZER.exe for the portable version.
3. Optional: run INSTALL_SAWCHA_OPTIMIZER.bat to create Desktop and Start Menu
   shortcuts under your user account. No administrator install is required.
4. After launch, watch the red/gray loading screen while the automatic
   hardware, video driver, and game-library scan runs.
5. Sign in offline (no account needed), or configure a Google Desktop OAuth
   Client ID to use Google sign-in.
6. Open GAME LIBRARY to select an installed game, and then visit KEYBIND
   GUIDE, DPI + SENSITIVITY, and FRAME TIME LAB.

BUILT-IN MODULES
- Live resource monitoring plus automatic AMD/Intel CPU, NVIDIA/other GPU, RAM, disks, Windows, mouse,
  controller, network card and game discovery (Steam, Epic, common launchers)
- Red/gray native startup animation and dashboard
- Competitive/Balanced/Creator presets with configuration backup and restore
- Game library, add custom game .exe, launch selected game
- Selected-game keybinds, controller guidance, popular sensitivity starting
  points, saved per-game DPI/polling rate/sensitivity (manual DPI entry)
- Frame-time benchmarking using separately installed Intel PresentMon console
- Offline troubleshooting assistant; online OpenAI support with optional
  user-entered API key, explicitly initiated by the user and never stored
- Network ping, adapter details, Windows SFC/DISM/CHKDSK, logs and diagnostics
- Plain-language explanation library: what a tweak does, benefits, risks,
  and undo instructions
- Developer passphrase + protected diagnostic report export
- Native Google OAuth desktop PKCE and signed ID-token verification
- SHA-256 code-signing and verification helper scripts (requires YOUR cert)

IMPORTANT LIMITATIONS
- Windows 10/11 x64 only. GPU sensors require NVIDIA utilities/compatible drivers.
- True hardware sensor DPI is not generally exposed to Windows: enter the DPI
  reported by the mouse itself or manufacturer software. Windows mouse speed
  is not hardware DPI.
- Game keybinds are recommendation guides only. This app doesn't alter a
  game's configuration or run input macros.
- Not all games can be auto-discovered (e.g. some Game Pass installs). Add
  an executable manually if it is not detected.
- Actual in-game FPS capture requires installing PresentMon separately.
- No automatic overclock, anticheat bypass, system service killing, antivirus
  disabling, or risky universal driver tweaking.
- Offline assistant is a rules-based local helper. Online AI is only active
  when you explicitly choose it and supply an API key.
- No actual FPS increase is guaranteed. Compare frame times in equivalent
  scenes before deciding whether a setting was beneficial.
- This executable is UNSIGNED. To sign with your real verified identity, you
  must have a legitimate Windows-trusted Code Signing certificate (including
  its private key) in your own Windows certificate store. See SIGNING_README.
  Do NOT disable Windows Defender or add blanket exclusions just to run it.

YOUR DATA / SECURITY
Settings and saved benchmarks reside at:
  %LOCALAPPDATA%\Sawcha\Optimizer
A one-time random token protects the local 127.0.0.1 API and login is
session-based. The helper does not expose a public listening interface.
Google login requests OpenID, email and profile only. The password is entered
on Google's site. OAuth tokens are not stored on disk. Developer mode is a
separate password-protected feature, and not a Windows admin bypass.

TESTING
Windows binary cross-compiled for x64, and the engine unit tests and static
checks passed. This version has not been interactively tested on your exact
Windows hardware; report any error message with the exact steps that led to it.

SOURCES
The complete Go source code, with no .js files, is inside the SOURCE folder.
Build on Windows (Go 1.23+):
  cd SOURCE
  go build -ldflags="-H windowsgui" -o ..\SAWCHA_OPTIMIZER.exe .

Note: Build the EXE before running SIGN_WITH_CERTIFICATE.ps1. Rebuilding or
modifying a signed executable invalidates its Authenticode signature.

RENAME NOTES
- Customer-facing brand and new data folder are SAWCHA.
- For continuity, this build copies settings.json and benchmarks.json from the prior
  RageWeezy app data folder on first run if the Sawcha equivalents are absent.
- This build does not include hosted billing, a Stripe checkout integration, or
  a Plus license-activation interface. Do not market Plus as enabled yet.
