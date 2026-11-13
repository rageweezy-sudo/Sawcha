# Optimizer feature review: ideas adopted safely

Comparisons below are descriptions of public documented features, **not copied code or proprietary algorithms**. Avoid false claims about CPU affinity, registry magic or performance outcomes without real benchmarks.

| Reference product | Published idea | Sawcha implementation | Status |
|---|---|---|---|
| Razer Cortex Game Booster | Recognizes a game and uses a game-aware mode; returns to normal when gaming ends | Auto-detect only registered games, cut **Sawcha's own** polling/drawing overhead, restore normal monitoring when game exits | Implemented in v8 |
| Process Lasso | Detect games, prioritize keeping background resource use from disrupting foreground processes | Game-process monitoring and user-visible warning; intentionally does **not** force process priority/affinity | Implemented selectively |
| NVIDIA App | Driver info, per-game settings and monitoring | NVIDIA telemetry and links to official driver/control panels; game advice as non-automatic guidance | Existing |
| Windows built-in Task Manager/Windows Settings | Processes and memory use, startup settings, Game Mode | Live process summaries, manual links to device/startup/game graphics settings, reversible Game Mode presets | Existing |
| Intel PresentMon | Actual presentation timings, FPS and 1% lows | Optional capture and frame-time benchmark comparison using separately installed PresentMon | Existing, external dependency |
| Microsoft PC Manager | Storage, startup and health information | Windows tools and explanations; no auto-deletion of unrelated user data | Partial; intentionally conservative |

Primary references (official):
- https://www.razer.com/cortex/booster
- https://bitsum.com/apps/process-lasso/docs/algorithms/performance-mode/
- https://bitsum.com/apps/process-lasso/docs/getting-started/setup-guide/
- https://nvidia.custhelp.com/app/answers/detail/a_id/5521/~/nvidia-app-faq
- https://github.com/GameTechDev/PresentMon

Safety design: refrain from injecting DLLs into anti-cheat protected games, disabling security services, ending unknown processes automatically, forcing Real-time process priority, deleting files without preview, or applying GPU/CPU voltage/clock changes without manufacturer-safe controls and measurements.
