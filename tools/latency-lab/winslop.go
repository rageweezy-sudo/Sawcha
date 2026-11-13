package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// The audit is an original SAWCHA implementation inspired by Windows cleanup
// categories. No code or binary from Winslop/Winslopr/FluentTweaker is bundled.
// This feature ONLY reads configuration. It has no mutation API.
const fluentTweakerReleases = "https://github.com/builtbybel/FluentTweaker/releases"

type WinSlopFinding struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	State       string `json:"state"`
	Evidence    string `json:"evidence"`
	Explanation string `json:"explanation"`
	Performance string `json:"performance"`
}

type WinSlopAudit struct {
	TimestampUTC string           `json:"timestampUtc"`
	Mode         string           `json:"mode"`
	Upstream     string           `json:"upstream"`
	OfficialURL  string           `json:"officialUrl"`
	ModifiedOS   bool             `json:"modifiedOS"`
	Findings     []WinSlopFinding `json:"findings"`
	Warnings     []string         `json:"warnings"`
	Disclaimer   string           `json:"disclaimer"`
}

type slopCheck struct {
	id, category, title, explanation, performance string
}

var slopChecks = []slopCheck{
	{"gameDvr", "Gaming", "Game DVR background capture", "Whether a per-user Game DVR registry preference is explicitly set.", "Background recording can consume resources while active. A setting alone does not prove lower latency."},
	{"appCapture", "Gaming", "Game capture preference", "Whether the Windows Game DVR app capture preference is explicitly set.", "Keep recording if you use clips or capture tools. Measure same-scene frame pacing."},
	{"widgets", "Taskbar", "Windows Widgets taskbar", "Whether the Windows 11 taskbar Widgets control is explicitly set.", "UI preference, not a measured input-latency optimization."},
	{"tailored", "Privacy", "Tailored experiences", "Whether the per-user tailoring setting is explicitly configured.", "Privacy preference, not an established gaming-latency fix."},
	{"consumer", "Suggestions", "Windows consumer features policy", "Whether an administrator policy about consumer features is explicitly set.", "Policy changes may affect managed computers; no blanket performance claim."},
	{"softLanding", "Suggestions", "Windows tips and suggestions", "Whether the ContentDeliveryManager SoftLanding preference is set.", "Mostly cosmetic. Disabling tips does not guarantee an FPS increase."},
	{"startSuggestions", "Suggestions", "Suggested content in Start", "Whether the Start suggestions preference is explicitly set.", "Changing recommendations is a UI preference, not a proven latency reduction."},
	{"lockScreenTips", "Suggestions", "Lock-screen tips", "Whether the lock-screen suggestions preference is explicitly set.", "Mostly cosmetic; do not treat as a latency fix."},
	{"searchSuggestions", "Search", "Search box web suggestions policy", "Whether the Explorer policy for Search suggestions is configured.", "A search privacy choice. Search usability can change if disabled."},
	{"copilotPolicy", "AI features", "Copilot policy", "Whether Windows Copilot's policy is explicitly set.", "AI feature availability may be managed by your organization; do not infer running overhead from this policy."},
	{"recallPolicy", "AI features", "Recall snapshot policy", "Whether the Windows AI policy for Recall snapshotting is explicitly configured.", "This does not prove Recall is installed or recording. Inspect Windows settings before changing anything."},
	{"advertisingId", "Privacy", "Advertising ID", "Whether Windows advertising ID is explicitly enabled or disabled by user preference.", "Privacy preference; no measured gaming latency effect."},
}

var optionalAppNames = []struct{ ID, Title string }{
	{"Microsoft.Copilot", "Microsoft Copilot"},
	{"Microsoft.MicrosoftSolitaireCollection", "Solitaire Collection"},
	{"Clipchamp.Clipchamp", "Clipchamp"},
	{"Microsoft.WindowsFeedbackHub", "Feedback Hub"},
	{"Microsoft.BingNews", "Microsoft News"},
	{"Microsoft.GetHelp", "Get Help"},
	{"Microsoft.YourPhone", "Phone Link"},
	{"Microsoft.XboxGamingOverlay", "Xbox Game Bar"},
}

// buildWinSlopAudit only interprets data that was read by Windows. A missing
// key remains unknown: no guessing at Microsoft defaults or installed status.
func buildWinSlopAudit(snapshot map[string]any) WinSlopAudit {
	audit := WinSlopAudit{
		TimestampUTC: time.Now().UTC().Format(time.RFC3339),
		Mode:         "read-only inspection; no remediation or script execution",
		Upstream:     "WinSlop / Winslopr (renamed FluentTweaker) by builtbybel; separate upstream application",
		OfficialURL:  fluentTweakerReleases,
		ModifiedOS:   false,
		Findings:     make([]WinSlopFinding, 0, len(slopChecks)+len(optionalAppNames)),
		Warnings:     []string{},
		Disclaimer:   "No Windows settings, apps, drivers, security policies or services were modified. Registry values show only what is explicitly configured, not effective policy or performance gain. Third-party tool is not bundled or launched.",
	}
	observed, _ := snapshot["registry"].(map[string]any)
	for _, check := range slopChecks {
		state := "Not explicitly configured / unreadable"
		evidence := "No readable registry value found; Windows defaults vary by version and policy."
		if value, found := observed[check.id]; found && value != nil {
			state = "Explicit value found"
			evidence = fmt.Sprintf("Registry value = %v (interpret with Windows build and policy)", value)
		}
		audit.Findings = append(audit.Findings, WinSlopFinding{
			ID: check.id, Category: check.category, Title: check.title,
			State: state, Evidence: evidence, Explanation: check.explanation, Performance: check.performance,
		})
	}
	packages, _ := snapshot["installedOptionalApps"].([]any)
	found := map[string]bool{}
	for _, p := range packages {
		name, ok := p.(string)
		if !ok {
			continue
		}
		for _, known := range optionalAppNames {
			if strings.EqualFold(known.ID, name) {
				found[strings.ToLower(name)] = true
			}
		}
	}
	for _, app := range optionalAppNames {
		state, evidence := "Not detected for current user", "A missing current-user AppX package is not proof of absence for all accounts."
		if found[strings.ToLower(app.ID)] {
			state, evidence = "Installed for current user", "AppX package identified by exact allowlisted package name."
		}
		audit.Findings = append(audit.Findings, WinSlopFinding{
			ID: "app:" + app.ID, Category: "Optional apps", Title: app.Title, State: state,
			Evidence:    evidence,
			Explanation: "Inventory only; no uninstallation is offered or performed.",
			Performance: "Installed does not mean running. Removing an app is not a proven latency optimization.",
		})
	}
	if count, ok := snapshot["userStartupEntryCount"].(float64); ok && count >= 0 && count <= 10000 {
		audit.Findings = append(audit.Findings, WinSlopFinding{
			ID: "startup", Category: "Startup", Title: "User startup entries",
			State:       fmt.Sprintf("%d entries observed", int(count)),
			Evidence:    "Counts entries only; no executable paths or commands are collected.",
			Explanation: "Review startup apps yourself in Windows Task Manager.",
			Performance: "Unneeded startup processes may use memory/CPU; entry count alone cannot establish latency.",
		})
	}
	return audit
}

func (s *server) winSlopInspectHandler(w http.ResponseWriter, r *http.Request) {
	var request struct{}
	if err := decodeJSON(r, &request, 1024); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	if !s.workMu.TryLock() {
		errorJSON(w, 409, "Another diagnostic operation is in progress")
		return
	}
	defer s.workMu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	snapshot, err := readWinSlopSnapshot(ctx)
	if err != nil {
		errorJSON(w, 500, "Read-only WinSlop inspection failed: "+err.Error())
		return
	}
	audit := buildWinSlopAudit(snapshot)
	s.mu.Lock()
	s.latestSlop = &audit
	if s.latest != nil {
		clone := *s.latest // never mutate a report being read by another handler
		clone.WinSlopAudit = &audit
		s.latest = &clone
	}
	s.mu.Unlock()
	sendJSON(w, audit)
}
