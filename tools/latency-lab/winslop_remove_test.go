package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestRemovalAllowlistNeverAcceptsWindowsCoreOrInjectedIdentifiers(t *testing.T) {
	bad := []string{"Microsoft.WindowsStore", "Microsoft.Windows.ShellExperienceHost", "Microsoft.Windows.StartMenuExperienceHost", "Microsoft.Windows.SecHealthUI", "Microsoft.GamingServices", "Microsoft.MicrosoftEdge.Stable", "Microsoft.DesktopAppInstaller", "Microsoft.Copilot; Remove-Item C:\\", "Microsoft.Copilot,Microsoft.XboxGamingOverlay", ""}
	for _, id := range bad {
		if _, err := removalSelection([]string{id}); err == nil {
			t.Fatalf("dangerous name accepted: %q", id)
		}
	}
	if _, err := removalSelection([]string{"Microsoft.Copilot", "Microsoft.Copilot"}); err == nil {
		t.Fatal("duplicate ID accepted")
	}
	ids := make([]string, 0, 7)
	for _, app := range removableApps[:7] {
		ids = append(ids, app.ID)
	}
	if _, err := removalSelection(ids); err == nil {
		t.Fatal("unbounded bulk removal allowed")
	}
	apps, err := removalSelection([]string{"Microsoft.Copilot", "Clipchamp.Clipchamp"})
	if err != nil || len(apps) != 2 || removalIDs(apps) != "Microsoft.Copilot,Clipchamp.Clipchamp" {
		t.Fatalf("selection failed: %+v %v", apps, err)
	}
}
func TestRemovalRequiresExplicitConfirmations(t *testing.T) {
	app := makeTestServer()
	for _, body := range []string{
		`{}`,
		`{"ids":["Microsoft.Copilot"]}`,
		`{"ids":["Microsoft.Copilot"],"confirmDataLoss":true}`,
		`{"ids":["Microsoft.Copilot"],"confirmDataLoss":false,"confirmation":"REMOVE SELECTED APPS"}`,
		`{"ids":["Microsoft.Copilot"],"confirmDataLoss":true,"confirmation":"REMOVE"}`,
		`{"ids":["Microsoft.WindowsStore"],"confirmDataLoss":true,"confirmation":"REMOVE SELECTED APPS"}`,
		`{"ids":["Microsoft.Copilot"],"confirmDataLoss":true,"confirmation":"REMOVE SELECTED APPS","extraCode":"calc.exe"}`,
	} {
		out := callAPI(app, "/api/winslop/remove", body, "unit-test-token", app.origin)
		if out.Code != http.StatusBadRequest {
			t.Fatalf("unsafe removal was allowed (%d): %s", out.Code, out.Body.String())
		}
	}
}
func TestRemovalPlanIsReadOnlyAndIncludesScopeAndWarnings(t *testing.T) {
	app := makeTestServer()
	w := callAPI(app, "/api/winslop/plan", `{"ids":["Microsoft.XboxGamingOverlay","Microsoft.YourPhone"]}`, "unit-test-token", app.origin)
	if w.Code != http.StatusOK {
		t.Fatalf("plan: %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Win+G") || !strings.Contains(body, "current Windows user only") || !strings.Contains(body, "REMOVE SELECTED APPS") {
		t.Fatalf("missing destructive-action warning: %s", body)
	}
	if app.latestSlop != nil || app.latest != nil {
		t.Fatal("preview mutated report state")
	}
}
func TestRemovalRequiresSameOriginSessionToken(t *testing.T) {
	app := makeTestServer()
	body := `{"ids":["Microsoft.Copilot"],"confirmDataLoss":true,"confirmation":"REMOVE SELECTED APPS"}`
	for _, tc := range []struct{ token, origin string }{{"", app.origin}, {"unit-test-token", "https://attacker.invalid"}} {
		w := callAPI(app, "/api/winslop/remove", body, tc.token, tc.origin)
		if w.Code != http.StatusForbidden {
			t.Fatalf("unauthorized removal returned %d", w.Code)
		}
	}
}
func TestReadOnlyInspectionStaysReadOnly(t *testing.T) {
	a := buildWinSlopAudit(map[string]any{"installedOptionalApps": []any{"Microsoft.Copilot"}})
	if a.ModifiedOS || !strings.Contains(a.Mode, "read-only") {
		t.Fatal("audit was changed to apply changes")
	}
}
