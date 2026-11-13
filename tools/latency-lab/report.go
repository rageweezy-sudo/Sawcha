package main

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"time"
)

type Report struct {
	FormatVersion int            `json:"formatVersion"`
	TimestampUTC  string         `json:"timestampUtc"`
	Scope         string         `json:"scope"`
	Architecture  string         `json:"architecture"`
	WindowsData   map[string]any `json:"windowsData"`
	WinSlopAudit  *WinSlopAudit  `json:"windowsSlopAudit,omitempty"`
	Warnings      []string       `json:"warnings"`
	Assessment    []string       `json:"assessment"`
	Privacy       string         `json:"privacy"`
}

func collectReport(ctx context.Context, deep bool) Report {
	out := Report{FormatVersion: 1, TimestampUTC: time.Now().UTC().Format(time.RFC3339), Scope: "On-demand, read-only Windows 10/11 performance diagnostic", Architecture: runtime.GOARCH, WindowsData: map[string]any{}, Warnings: []string{}, Assessment: []string{}, Privacy: "Does not collect hostnames, user accounts, IP or MAC addresses, disk serial numbers, raw command lines, browser history, files, or credentials. Driver names, app names and hardware models are included. Kernel ETL captures are separate and may contain sensitive data."}
	d, err := windowsSnapshot(ctx, deep)
	if err != nil {
		out.Warnings = append(out.Warnings, err.Error())
	}
	for k, v := range d {
		out.WindowsData[k] = v
	}
	out.Assessment = assessment(out)
	return out
}
func assessment(report Report) []string {
	tips := []string{"No system changes were made. Compare identical game scenes with SAWCHA off/on before concluding that latency improved.", "For kernel DPC/ISR, scheduling, disk I/O and interrupt timing, choose the optional 12-second WPR capture; inspect the ETL locally using Windows Performance Analyzer."}
	cpu, ok := report.WindowsData["cpu"].(map[string]any)
	if ok {
		if x, ok := number(cpu["LoadPercentage"]); ok && x > 85 {
			tips = append(tips, "CPU load exceeds 85% during this snapshot. Check what was running before adjusting Windows priorities.")
		}
	}
	osinfo, ok := report.WindowsData["os"].(map[string]any)
	if ok {
		if total, good := number(osinfo["TotalVisibleMemorySize"]); good && total > 0 {
			if avail, good := number(osinfo["FreePhysicalMemory"]); good && avail/total < .12 {
				tips = append(tips, "Less than 12% of physical memory was available: check memory pressure and background apps.")
			}
		}
	}
	if len(report.Warnings) > 0 {
		tips = append(tips, "Some optional Windows information was unavailable. Missing metrics are not treated as zero or as a system failure.")
	}
	return tips
}
func number(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case json.Number:
		f, e := x.Float64()
		return f, e == nil
	default:
		return 0, false
	}
}
func offlineAdvice(q string, r *Report) string {
	q = strings.TrimSpace(q)
	if r == nil {
		return "Run a diagnostic scan first. The app can then provide offline checks or optional AI assistance with explicit consent."
	}
	text := "SAWCHA offline diagnostic review (not a medical or hardware guarantee):\n"
	for _, s := range r.Assessment {
		text += "- " + s + "\n"
	}
	l := strings.ToLower(q)
	if strings.Contains(l, "mouse") || strings.Contains(l, "input") {
		text += "- Mouse: check hardware polling, USB stability and click-to-photon tests. Pointer speed changes are not a proven latency improvement.\n"
	}
	if strings.Contains(l, "audio") || strings.Contains(l, "microphone") {
		text += "- Audio: test round-trip latency and glitches with effects bypassed, and monitor for DPC/ISR spikes; avoid unmeasured buffering.\n"
	}
	if strings.Contains(l, "fps") || strings.Contains(l, "game") {
		text += "- Games: compare PresentMon P99 frame times and 1% lows in the same scene; do not equate higher average FPS with lower click-to-photon delay.\n"
	}
	return text + "Offline mode does not communicate with ChatGPT. For AI analysis, enter your own API key and explicitly choose which report data to send."
}
func reportForAI(r *Report, include bool) string {
	if !include || r == nil {
		return "No diagnostic report supplied. Do not assume access to this computer."
	}
	// Data is allowlisted by the collector; still cap bytes for consent and cost.
	b, _ := json.Marshal(r)
	if len(b) > 26000 {
		return string(b[:26000]) + "\n[REPORT TRUNCATED]"
	}
	return string(b)
}
func contextWithBound(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, d)
}
