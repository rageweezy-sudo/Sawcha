package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSawchaGameGuides(t *testing.T) {
	samples := map[string]string{
		"Rainbow Six Siege": "siege",
		"Siege X":           "siege",
		"Counter-Strike 2":  "cs2",
		"Fortnite":          "fortnite",
		"VALORANT":          "valorant",
	}
	for name, want := range samples {
		g := chooseGuide(name)
		if g.ID != want {
			t.Errorf("%q got guide %q, wanted %q", name, g.ID, want)
		}
		if len(g.Binds) < 4 || len(g.Setups) < 2 || g.VideoNotes == "" || g.ControllerNotes == "" {
			t.Errorf("%q guide missing actionable sections", name)
		}
	}
	if len(chooseGuide("An unknown indie game").Binds) == 0 {
		t.Fatal("missing generic fallback")
	}
}

func TestSawchaOfflineAssistantRoutes(t *testing.T) {
	tests := map[string]string{
		"My game has stutters":                "frame",
		"Network ping is high":                "router",
		"How do I choose mouse DPI?":          "sensor DPI",
		"Windows blue screen and GPU crash":   "overclock",
		"What about keybinds for controller?": "Keybind",
		"My PC is hot and throttles":          "temperatures",
		"How do I record in OBS?":             "Creator",
	}
	for question, want := range tests {
		got := offlineAnswer(question, "Rainbow Six Siege", map[string]interface{}{}, nil, nil)
		if !strings.Contains(strings.ToLower(got), strings.ToLower(want)) {
			t.Errorf("assistant answer for %q lacks expected %q", question, want)
		}
	}
}

func TestSawchaLegacyPreferenceImport(t *testing.T) {
	base := t.TempDir()
	legacy := filepath.Join(base, "RageWeezy", "Optimizer")
	newest := filepath.Join(base, "Sawcha", "Optimizer")
	if err := os.MkdirAll(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newest, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "settings.json"), []byte(`{"preset":"Balanced"}`), 0600); err != nil {
		t.Fatal(err)
	}
	importLegacySettings(newest)
	copied, err := os.ReadFile(filepath.Join(newest, "settings.json"))
	if err != nil || !strings.Contains(string(copied), "Balanced") {
		t.Fatal("preferences were not imported")
	}
	if err := os.WriteFile(filepath.Join(newest, "settings.json"), []byte(`{"preset":"Creator"}`), 0600); err != nil {
		t.Fatal(err)
	}
	importLegacySettings(newest)
	copied, _ = os.ReadFile(filepath.Join(newest, "settings.json"))
	if !strings.Contains(string(copied), "Creator") {
		t.Fatal("existing Sawcha settings overwritten")
	}
}
