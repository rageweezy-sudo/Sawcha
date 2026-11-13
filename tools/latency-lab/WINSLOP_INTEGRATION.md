# SAWCHA optional Windows app removal

The integration has moved from **inspection only** to **actual current-user AppX removal** (see `README.md`). Removal is always manually selected and explicitly confirmed. No third-party source or executable is bundled or changed.

- Inspect existing Windows app and selected policy state with read-only PowerShell.
- Select up to six allowlisted optional current-user AppX applications.
- Preview each selected app's warnings, acknowledge potential app data loss, type the required confirmation, and approve the final confirmation dialogue.
- Resolve exact installed package identities inside PowerShell only; block protected, framework, resource and system apps; use `Remove-AppxPackage` without `-AllUsers`; verify removal and rescan.
- Keep core OS, Windows security, Microsoft Store, Edge/WebView2 and Gaming Services untouched. No arbitrary commands, debloating scripts, service removal, policy tweaking, background cleanup or persistent OS modifications besides specifically requested AppX removals.
- No claim of latency improvement: compare performance before/after; removing an installed app may have no effect if it was not running.
- No guaranteed rollback; Microsoft Store reinstall may be needed and local app data can be lost.

Source inspiration/attribution: https://github.com/builtbybel/FluentTweaker/releases . This app is **not** a repackaged FluentTweaker binary and contains no redistributed FluentTweaker source.
