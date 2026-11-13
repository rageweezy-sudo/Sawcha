package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AppPreferences control SAWCHA itself, not the game or Windows security services.
type AppPreferences struct {
	DetectGames         bool `json:"detect_games"`
	QuietWhileGaming    bool `json:"quiet_while_gaming"`
	WarnBeforeChanges   bool `json:"warn_before_changes"`
	LatencySafe         bool `json:"latency_safe"`
	QuietMonitorSeconds int  `json:"quiet_monitor_seconds"`
}

func defaultAppPreferences() AppPreferences {
	return AppPreferences{DetectGames: true, QuietWhileGaming: true, WarnBeforeChanges: true, LatencySafe: true, QuietMonitorSeconds: 90}
}
func validPreferences(p AppPreferences) bool {
	return p.QuietMonitorSeconds == 60 || p.QuietMonitorSeconds == 90 || p.QuietMonitorSeconds == 120
}
func (a *App) loadAppPreferences() {
	p := defaultAppPreferences()
	b, e := os.ReadFile(filepath.Join(a.root, "app_preferences.json"))
	if e == nil {
		_ = json.Unmarshal(b, &p)
	}
	if !validPreferences(p) {
		p.QuietMonitorSeconds = 90
	}
	a.mu.Lock()
	a.preferences = p
	a.mu.Unlock()
}
func (a *App) writeAppPreferences(p AppPreferences) error {
	if !validPreferences(p) {
		return errors.New("invalid monitoring interval")
	}
	b, e := json.MarshalIndent(p, "", "  ")
	if e != nil {
		return e
	}
	file := filepath.Join(a.root, "app_preferences.json")
	if e = os.MkdirAll(a.root, 0700); e != nil {
		return e
	}
	tmp := file + ".tmp"
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	if e = os.Rename(tmp, file); e != nil {
		return e
	}
	a.mu.Lock()
	a.preferences = p
	if !p.DetectGames {
		a.runningGame = ""
	}
	a.mu.Unlock()
	return nil
}
func (a *App) guardSnapshot() (string, AppPreferences) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.runningGame, a.preferences
}
func (a *App) isQuietGaming() bool {
	g, p := a.guardSnapshot()
	return shouldSkipHeavyMonitor(g, p)
}
func (a *App) changeDuringGameWarning(ack bool) error {
	g, p := a.guardSnapshot()
	if g != "" && p.WarnBeforeChanges && !ack {
		return errors.New("Game running: " + g + ". Sawcha paused this system change to prevent stutter. Confirm the in-game warning to proceed or close the game first.")
	}
	return nil
}

// Known processes are only used if the game is already in the discovered library.
// Never treat Steam/Epic launchers themselves as running games.
var fallbackProcessNames = map[string][]string{
	"rainbow six siege": {"rainbowsix.exe", "rainbowsix_vulkan.exe"},
	"rainbow six":       {"rainbowsix.exe", "rainbowsix_vulkan.exe"},
	"fortnite":          {"fortniteclient-win64-shipping.exe"},
	"valorant":          {"valorant-win64-shipping.exe"},
	"counter-strike":    {"cs2.exe"},
	"apex legends":      {"r5apex.exe"},
	"rocket league":     {"rocketleague.exe"},
	"minecraft":         {"javaw.exe"},
	"overwatch":         {"overwatch.exe"},
	"call of duty":      {"cod.exe", "cod22-cod.exe"},
	"rust":              {"rustclient.exe"},
}
var ignoreExe = map[string]bool{"steam.exe": true, "epicgameslauncher.exe": true, "launcher.exe": true, "uninstall.exe": true, "setup.exe": true, "unins000.exe": true, "unitycrashhandler64.exe": true, "crashreportclient.exe": true, "game.exe": true, "java.exe": true}

func processBasename(path string) string {
	if idx := strings.LastIndexAny(path, `\/`); idx >= 0 {
		path = path[idx+1:]
	}
	return strings.ToLower(strings.TrimSpace(path))
}
func selectRunningGame(games []Game, running map[string]bool) string {
	for _, g := range games {
		ex := processBasename(g.Exe)
		if len(ex) > 4 && strings.HasSuffix(ex, ".exe") && !ignoreExe[ex] && running[ex] {
			return g.Name
		}
		name := strings.ToLower(g.Name)
		for known, list := range fallbackProcessNames {
			if strings.Contains(name, known) {
				for _, exe := range list {
					if running[exe] {
						return g.Name
					}
				}
			}
		}
	}
	return ""
}

// Check for game process every 15s; never inject, elevate, or inspect game memory.
func (a *App) gameWatchLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	scan := func() {
		a.mu.RLock()
		prefs := a.preferences
		games := append([]Game(nil), a.games...)
		a.mu.RUnlock()
		if !prefs.DetectGames {
			a.mu.Lock()
			a.runningGame = ""
			a.mu.Unlock()
			return
		}
		procs, err := runningProcesses()
		if err != nil {
			return
		} // Do not falsely claim none on a transient Windows error.
		found := selectRunningGame(games, procs)
		a.mu.Lock()
		prev := a.runningGame
		a.runningGame = found
		a.mu.Unlock()
		if prev != found {
			if found != "" {
				a.logger("Game detected: " + found + "; Sawcha background usage reduced when enabled")
			} else {
				a.logger("Game closed; Sawcha normal monitoring restored")
			}
		}
	}
	scan()
	for range ticker.C {
		scan()
	}
}
