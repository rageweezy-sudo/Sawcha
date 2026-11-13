# Windows cleanup inspection — provenance and no-change contract

SAWCHA Latency Lab integrates **read-only inspection categories** similar to WinSlop / Winslopr, a project by [builtbybel](https://github.com/builtbybel) which later became [FluentTweaker](https://github.com/builtbybel/FluentTweaker). This is an independent SAWCHA implementation. We did not import, copy, alter, bundle, build, download, or execute upstream source, plug-ins, scripts, images, or binaries. The current upstream GitHub repository did not advertise a license permitting us to redistribute code when checked on 2026-11-13. Do not assume its public availability grants reuse rights.

The only upstream integration is a hyperlink to [the publisher's official GitHub releases](https://github.com/builtbybel/FluentTweaker/releases). The user independently chooses if, when and how to download or run it. Do not use unrelated lookalike domains.

## Scope of this original inspector

With the user's explicit click, SAWCHA runs a fixed local read-only PowerShell script that queries selected Windows configuration values (Game DVR, Widgets, privacy and personalization, search suggestions, Copilot/Recall policy), a small allowlisted set of optional Windows AppX package names **for the current user only**, and the count of HKCU Run entries. It does not collect registry command lines, startup executable paths, app data, browser history, IP addresses, passwords, licence keys, user names or system files. Missing settings remain **unknown**, not inferred as enabled or disabled.

Observations indicate only current readable values, not actual running overhead or end-to-end performance. Installed apps may be dormant; a visible registry policy does not prove whether a feature is available on the installed Windows edition/build. You must measure latency separately.

## Hard restrictions

- No remediation/Apply/Remove API endpoint.
- No `Set-ItemProperty`, `reg add`, `reg delete`, `Remove-AppxPackage`, `winget uninstall`, DISM feature changes, service changes, task scheduling or driver install.
- No remote script download, `iex`, executable launch, automated update, scheduled monitoring, registry or filesystem modification.
- Inspection shares the single-operation lock with WPR tracing and heavy system scans.
- Data stays local unless the user separately selects report-sharing for an OpenAI request. ETL files remain excluded from reports.
- Existing SAWCHA optimizer and Latency Lab features must remain unchanged, except for adding this isolated page/report field. No silent changes to latency protections, kernel tracing or mic/audio routing.
- Windows 10/11 x64 and Intel/AMD are intended targets. Real machine validation still required.

## If a user wants actual cleanup

Open the publisher's GitHub release page using the read-only link and review the independent publisher's program, each operation, and its backups. The SAWCHA inspector never submits cleanup operations and **does not claim any latency improvement from removing software**. Keep security, Defender, Windows Update, and driver signing protections enabled.
