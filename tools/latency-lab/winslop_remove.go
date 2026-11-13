package main

import (
	"context"
	"errors"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// Original SAWCHA removal logic. No Winslop/FluentTweaker code is bundled.
// The allowlist deliberately excludes OS frameworks, Store, security, Gaming
// Services, Edge/WebView2, WinGet and system shell components.
const removalConfirmation = "REMOVE SELECTED APPS"
const removalBatchLimit = 6

type optionalApp struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Warning string `json:"warning"`
}

var removableApps = []optionalApp{
	{"Microsoft.Copilot", "Microsoft Copilot", "Copilot app shortcuts and app data may disappear. This does not remove Windows AI components or enterprise policies."},
	{"Microsoft.MicrosoftSolitaireCollection", "Solitaire Collection", "Game progress or locally saved app data may be lost."},
	{"Clipchamp.Clipchamp", "Clipchamp", "Unsaved editing projects or locally stored app data may be lost."},
	{"Microsoft.WindowsFeedbackHub", "Feedback Hub", "You will no longer have the Feedback Hub application until reinstalled."},
	{"Microsoft.BingNews", "Microsoft News", "News preferences and locally stored app data may be lost."},
	{"Microsoft.GetHelp", "Get Help", "The Windows Get Help application will be unavailable until reinstalled."},
	{"Microsoft.YourPhone", "Phone Link", "PC-to-phone linking features may stop working; pairing may require setup again."},
	{"Microsoft.XboxGamingOverlay", "Xbox Game Bar", "WARNING: removing Game Bar can break Win+G, FPS overlays, captures and game clips. Do not remove if you use these features."},
	{"Microsoft.BingWeather", "MSN Weather", "Saved weather locations and preferences may be lost."},
	{"Microsoft.WindowsMaps", "Windows Maps", "Offline maps and app preferences may be lost."},
	{"Microsoft.People", "People", "The People app will be removed; underlying account contacts are not targeted."},
	{"Microsoft.ZuneMusic", "Media Player / Groove", "Media Player / Groove functionality may be removed. Music files are not targeted."},
	{"Microsoft.ZuneVideo", "Movies & TV", "The Movies & TV app will be removed. Purchased content availability may change."},
	{"Microsoft.Getstarted", "Windows Tips", "The Windows Tips app will be removed where available."},
	{"Microsoft.549981C3F5F10", "Cortana (legacy)", "The legacy Cortana app will be removed where available."},
	{"Microsoft.OutlookForWindows", "Outlook for Windows", "The new Outlook app will be removed; ensure mail is synchronized before uninstalling."},
}

type removalPlan struct {
	Apps         []optionalApp `json:"apps"`
	Scope        string        `json:"scope"`
	Confirmation string        `json:"confirmation"`
	Warning      string        `json:"warning"`
}

type appRemovalResult struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type appRemovalReceipt struct {
	TimestampUTC string             `json:"timestampUtc"`
	Scope        string             `json:"scope"`
	Results      []appRemovalResult `json:"results"`
	Note         string             `json:"note"`
}

func removalSelection(ids []string) ([]optionalApp, error) {
	if len(ids) == 0 || len(ids) > removalBatchLimit {
		return nil, errors.New("select 1 to 6 optional apps per removal operation")
	}
	result := make([]optionalApp, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			return nil, errors.New("duplicate app IDs are not allowed")
		}
		seen[id] = true
		allowed := false
		for _, app := range removableApps {
			if app.ID == id {
				result = append(result, app)
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, errors.New("a selected item is not a supported optional app; Windows core and security packages are protected")
		}
	}
	return result, nil
}

func (s *server) winSlopPlanHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ids []string `json:"ids"`
	}
	if err := decodeJSON(r, &req, 4096); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	apps, err := removalSelection(req.Ids)
	if err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	sendJSON(w, removalPlan{Apps: apps, Scope: "current Windows user only; does not remove provisioned packages or apps from other accounts", Confirmation: removalConfirmation, Warning: "Selected apps will actually be uninstalled. Personal app settings/data may be lost. No automatic rollback or latency improvement is guaranteed. Reinstall separately through Microsoft Store when available. Close these apps before removing them."})
}

func (s *server) winSlopRemoveHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs             []string `json:"ids"`
		ConfirmDataLoss bool     `json:"confirmDataLoss"`
		Confirmation    string   `json:"confirmation"`
	}
	if err := decodeJSON(r, &req, 4096); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	apps, err := removalSelection(req.IDs)
	if err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	if !req.ConfirmDataLoss || req.Confirmation != removalConfirmation {
		errorJSON(w, 400, "Explicit data-loss acknowledgement and exact removal confirmation are required")
		return
	}
	if runtime.GOOS != "windows" {
		errorJSON(w, 501, "Actual app removal requires Windows 10 or 11")
		return
	}
	if !s.workMu.TryLock() {
		errorJSON(w, 409, "Another scan, trace, or cleanup operation is in progress")
		return
	}
	defer s.workMu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 48*time.Second)
	defer cancel()
	receipt, err := removeWindowsOptionalApps(ctx, apps)
	if err != nil {
		errorJSON(w, 500, "Windows could not complete the removal operation: "+err.Error())
		return
	}
	// Results are returned to the browser for user-controlled export. Do not
	// automatically upload or store a removal history on disk.
	receipt.TimestampUTC = time.Now().UTC().Format(time.RFC3339)
	receipt.Scope = "current Windows user only"
	receipt.Note = "Removal results were checked against current-user AppX package registration. Packages for other users or provisioned future users were not targeted. App data may be lost and reinstallation is not guaranteed. Check Windows Settings and restart apps if needed."
	s.mu.Lock()
	// Previous inspection is now stale; do not represent it as post-removal proof.
	s.latestSlop = nil
	if s.latest != nil {
		clone := *s.latest
		clone.WinSlopAudit = nil
		s.latest = &clone
	}
	s.mu.Unlock()
	sendJSON(w, receipt)
}

// The fixed Windows script receives only previously validated, exact
// allowlisted IDs; never a user-controlled shell fragment or package path.
func removalIDs(apps []optionalApp) string {
	ids := make([]string, 0, len(apps))
	for _, a := range apps {
		ids = append(ids, a.ID)
	}
	return strings.Join(ids, ",")
}
