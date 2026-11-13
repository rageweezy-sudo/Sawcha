package main

import (
	"net/http"
)

// Local-only settings API. It does not alter Windows, games, or security services.
func (a *App) preferencesHandler(w http.ResponseWriter, r *http.Request) {
	var next AppPreferences
	if err := decode(r, &next); err != nil {
		bad(w, err.Error(), 400)
		return
	}
	if !validPreferences(next) {
		bad(w, "Quiet monitoring interval must be 60, 90 or 120 seconds", 400)
		return
	}
	if err := a.writeAppPreferences(next); err != nil {
		bad(w, "Unable to save preferences", 500)
		return
	}
	send(w, map[string]any{"ok": true, "preferences": next})
}
