package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestWinSlopReadOnlyWithoutInferredDefaults(t *testing.T) {
	snapshot := map[string]any{
		"registry":              map[string]any{"gameDvr": "1", "widgets": "0"},
		"installedOptionalApps": []any{"Microsoft.Copilot", "not.allowlisted.evil"},
		"userStartupEntryCount": float64(3),
	}
	a := buildWinSlopAudit(snapshot)
	if a.ModifiedOS || a.OfficialURL != fluentTweakerReleases || len(a.Findings) != len(slopChecks)+len(optionalAppNames)+1 {
		t.Fatalf("unsafe/missing audit: %+v", a)
	}
	if a.Findings[0].Evidence != "Registry value = 1 (interpret with Windows build and policy)" {
		t.Fatal(a.Findings[0])
	}
	if a.Findings[4].State != "Not explicitly configured / unreadable" {
		t.Fatal("missing policy was assumed: ", a.Findings[4])
	}
	if a.Findings[len(slopChecks)].State != "Installed for current user" {
		t.Fatal("known package missing")
	}
	for _, v := range a.Findings {
		if strings.Contains(v.ID, "evil") {
			t.Fatal("untrusted package was accepted")
		}
	}
	b, _ := json.Marshal(a)
	if strings.Contains(string(b), "commandline") || strings.Contains(string(b), "serialnumber") {
		t.Fatal("privacy regression")
	}
}
func TestWinSlopInspectionIsSessionAuthenticated(t *testing.T) {
	app := makeTestServer()
	for _, path := range []string{"/api/winslop/inspect", "/api/winslop/plan", "/api/winslop/remove"} {
		noToken := callAPI(app, path, "{}", "", app.origin)
		if noToken.Code != http.StatusForbidden {
			t.Fatalf("cleanup route unauthenticated: %s: %d", path, noToken.Code)
		}
	}
	for _, path := range []string{"/api/winslop/apply", "/api/winslop/remove-all", "/api/winslop/disable-defender"} {
		authorised := callAPI(app, path, "{}", "unit-test-token", app.origin)
		if authorised.Code != http.StatusNotFound {
			t.Fatalf("unsupported mutation endpoint %s: %d", path, authorised.Code)
		}
	}
}
func TestWinSlopInspectionHonoursExistingHeavyTaskLock(t *testing.T) {
	app := makeTestServer()
	app.workMu.Lock()
	defer app.workMu.Unlock()
	response := callAPI(app, "/api/winslop/inspect", "{}", "unit-test-token", app.origin)
	if response.Code != http.StatusConflict {
		t.Fatalf("audit overlapped kernel trace: %d", response.Code)
	}
}
