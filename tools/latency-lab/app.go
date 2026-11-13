package main

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var assets embed.FS

type server struct {
	token      string
	origin     string
	client     *http.Client
	mu         sync.RWMutex
	latest     *Report
	latestSlop *WinSlopAudit
	workMu     sync.Mutex
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" || r.Method != "GET" {
			http.NotFound(w, r)
			return
		}
		b, err := assets.ReadFile("web/index.html")
		if err != nil {
			http.Error(w, "UI missing", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		// Token is sent in page data on same-origin localhost, never in URL, logs or storage.
		html := strings.Replace(string(b), "__SAWCHA_SESSION_TOKEN__", s.token, 1)
		_, _ = io.WriteString(w, html)
	})
	mux.HandleFunc("/app.js", func(w http.ResponseWriter, r *http.Request) {
		serveAsset(w, r, "web/app.js", "text/javascript; charset=utf-8")
	})
	mux.HandleFunc("/style.css", func(w http.ResponseWriter, r *http.Request) {
		serveAsset(w, r, "web/style.css", "text/css; charset=utf-8")
	})
	api := func(path string, h http.HandlerFunc) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			if r.Method != "POST" {
				errorJSON(w, 405, "POST required")
				return
			}
			if r.Host != strings.TrimPrefix(s.origin, "http://") {
				errorJSON(w, 403, "Invalid localhost host")
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" && origin != s.origin {
				errorJSON(w, 403, "Foreign origin blocked")
				return
			}
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Sawcha-Token")), []byte(s.token)) != 1 {
				errorJSON(w, 403, "Session token required")
				return
			}
			h(w, r)
		})
	}
	api("/api/scan", s.scanHandler)
	api("/api/winslop/inspect", s.winSlopInspectHandler)
	api("/api/report", s.reportHandler)
	api("/api/chat", s.chatHandler)
	api("/api/trace", s.traceHandler)
	api("/api/local", s.localHandler)
	return mux
}
func serveAsset(w http.ResponseWriter, r *http.Request, path, mime string) {
	if r.Method != "GET" {
		http.Error(w, "GET required", 405)
		return
	}
	b, err := assets.ReadFile(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(b)
}
func sendJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(data)
}
func errorJSON(w http.ResponseWriter, status int, text string) {
	w.WriteHeader(status)
	sendJSON(w, map[string]string{"error": text})
}
func decodeJSON(r *http.Request, target any, limit int64) error {
	defer r.Body.Close()
	d := json.NewDecoder(io.LimitReader(r.Body, limit))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("extra JSON data not allowed")
	}
	return nil
}
func (s *server) scanHandler(w http.ResponseWriter, r *http.Request) {
	var options struct {
		Deep bool `json:"deep"`
	}
	if err := decodeJSON(r, &options, 1024); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	// Avoid overlapping heavy scans and kernel traces. No background collection.
	if !s.workMu.TryLock() {
		errorJSON(w, 409, "Another heavy diagnostic task is in progress")
		return
	}
	defer s.workMu.Unlock()
	report := collectReport(r.Context(), options.Deep)
	s.mu.Lock()
	report.WinSlopAudit = s.latestSlop
	s.latest = &report
	s.mu.Unlock()
	sendJSON(w, report)
}
func (s *server) reportHandler(w http.ResponseWriter, r *http.Request) {
	var opts struct{}
	if err := decodeJSON(r, &opts, 1024); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	s.mu.RLock()
	report := s.latest
	s.mu.RUnlock()
	if report == nil {
		errorJSON(w, 404, "Run a scan first")
		return
	}
	sendJSON(w, report)
}
func (s *server) localHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Question string `json:"question"`
	}
	if err := decodeJSON(r, &req, 8192); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	s.mu.RLock()
	report := s.latest
	s.mu.RUnlock()
	sendJSON(w, map[string]string{"answer": offlineAdvice(req.Question, report)})
}
func (s *server) traceHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConfirmSensitiveTrace bool `json:"confirmSensitiveTrace"`
	}
	if err := decodeJSON(r, &req, 1024); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	if !req.ConfirmSensitiveTrace {
		errorJSON(w, 400, "Explicit consent required: kernel ETW traces may contain paths and process information")
		return
	}
	if !s.workMu.TryLock() {
		errorJSON(w, 409, "Another heavy diagnostic task is in progress")
		return
	}
	defer s.workMu.Unlock()
	ctx, cancel := contextWithBound(r.Context(), 55*time.Second)
	defer cancel()
	result, err := captureKernelTrace(ctx, 12*time.Second)
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	sendJSON(w, map[string]any{"ok": true, "path": result, "notice": "ETL stays on this PC. It is NOT attached to your chat or diagnostic JSON. Analyze in Windows Performance Analyzer after reviewing its sensitive contents."})
}
