package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGameDetection(t *testing.T) {
	games := []Game{{Name: "Rainbow Six Siege", Exe: `C:\Games\Siege\RainbowSix.exe`}, {Name: "Fortnite", Exe: ""}, {Name: "Launcher", Exe: `C:\Games\launcher.exe`}}
	if v := selectRunningGame(games, map[string]bool{"rainbowsix.exe": true}); v != "Rainbow Six Siege" {
		t.Fatal(v)
	}
	if v := selectRunningGame(games, map[string]bool{"fortniteclient-win64-shipping.exe": true}); v != "Fortnite" {
		t.Fatal(v)
	}
	if v := selectRunningGame(games, map[string]bool{"launcher.exe": true}); v != "" {
		t.Fatal("launcher false positive")
	}
}
func TestGameGuardRequiresExplicitConsent(t *testing.T) {
	a := &App{root: t.TempDir(), preferences: defaultAppPreferences(), runningGame: "Fortnite"}
	if e := a.changeDuringGameWarning(false); e == nil {
		t.Fatal("missing warning")
	}
	if e := a.changeDuringGameWarning(true); e != nil {
		t.Fatal("user acknowledged", e)
	}
	a.preferences.WarnBeforeChanges = false
	if e := a.changeDuringGameWarning(false); e != nil {
		t.Fatal(e)
	}
}
func TestPreferencesPersistAndValidation(t *testing.T) {
	a := &App{root: filepath.Join(t.TempDir(), "config"), preferences: defaultAppPreferences(), runningGame: "Test"}
	p := defaultAppPreferences()
	p.DetectGames = false
	p.QuietMonitorSeconds = 120
	if e := a.writeAppPreferences(p); e != nil {
		t.Fatal(e)
	}
	b := &App{root: a.root}
	b.loadAppPreferences()
	_, saved := b.guardSnapshot()
	if saved.DetectGames || saved.QuietMonitorSeconds != 120 {
		t.Fatalf("not persisted %+v", saved)
	}
	if g, _ := a.guardSnapshot(); g != "" {
		t.Fatal("disabling detection did not reset mode")
	}
	p.QuietMonitorSeconds = 1
	if e := a.writeAppPreferences(p); e == nil {
		t.Fatal("bad interval accepted")
	}
	if _, e := os.Stat(filepath.Join(a.root, "app_preferences.json")); e != nil {
		t.Fatal(e)
	}
}
func TestPreferencesAPIRejectsInvalid(t *testing.T) {
	a := &App{root: t.TempDir(), preferences: defaultAppPreferences()}
	r := httptest.NewRequest(http.MethodPost, "/api/preferences", strings.NewReader(`{"detect_games":true,"quiet_while_gaming":true,"warn_before_changes":true,"quiet_monitor_seconds":1}`))
	w := httptest.NewRecorder()
	a.preferencesHandler(w, r)
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
func TestGameGuardBlocksSystemChangeEndpoints(t *testing.T) {
	a := &App{root: t.TempDir(), preferences: defaultAppPreferences(), runningGame: "Rainbow Six Siege", ready: true}
	cases := []struct {
		path, payload string
		fn            func(http.ResponseWriter, *http.Request)
	}{
		{"/api/preset", `{"profile":"Competitive","power":false}`, a.presetHandler},
		{"/api/restore", `{}`, a.restoreHandler},
		{"/api/diagnostic", `{"kind":"dns"}`, a.diagnosticHandler},
		{"/api/scan", `{}`, a.scanHandler},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.payload))
		tc.fn(w, req)
		if w.Code != 409 {
			t.Errorf("%s: expected game warning (409), got %d: %s", tc.path, w.Code, w.Body.String())
		}
	}
}
