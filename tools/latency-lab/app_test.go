package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func makeTestServer() *server {
	return &server{token: "unit-test-token", origin: "http://127.0.0.1:59808", client: &http.Client{Timeout: time.Second}}
}
func callAPI(app *server, path, payload, token, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "http://127.0.0.1:59808"+path, strings.NewReader(payload))
	r.Host = "127.0.0.1:59808"
	if token != "" {
		r.Header.Set("X-Sawcha-Token", token)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	return w
}
func TestAPIRequiresSessionTokenAndLocalOrigin(t *testing.T) {
	app := makeTestServer()
	for _, tc := range []struct{ token, origin string }{
		{"", ""}, {"incorrect", ""}, {"unit-test-token", "https://attacker.example"},
	} {
		w := callAPI(app, "/api/report", "{}", tc.token, tc.origin)
		if w.Code != 403 {
			t.Errorf("unauthorized call unexpectedly accepted: %d", w.Code)
		}
	}
	w := callAPI(app, "/api/report", "{}", "unit-test-token", app.origin)
	if w.Code != 404 {
		t.Fatalf("expected no report, got %d", w.Code)
	}
}
func TestUserInterfaceNeverEmbedsSecrets(t *testing.T) {
	app := makeTestServer()
	r := httptest.NewRequest("GET", "http://127.0.0.1:59808/", nil)
	r.Host = "127.0.0.1:59808"
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "unit-test-token") {
		t.Fatal("UI session token injection missing")
	}
	if strings.Contains(w.Body.String(), "sk-proj-") {
		t.Fatal("API credential accidentally embedded")
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "connect-src 'self'") {
		t.Fatal("no network restriction")
	}
}
func TestReportSharingRequiresExplicitOptIn(t *testing.T) {
	r := &Report{WindowsData: map[string]any{"cpu": map[string]any{"Name": "Example CPU"}}}
	if text := reportForAI(r, false); strings.Contains(text, "Example CPU") {
		t.Fatal("no-consent path leaked report")
	}
	if text := reportForAI(r, true); !strings.Contains(text, "Example CPU") {
		t.Fatal("consent path omitted report")
	}
}
func TestNoLeakOfAPIKeyInErrorResponse(t *testing.T) {
	app := makeTestServer()
	payload := `{"question":"Review GPU","apiKey":"sk-this-is-not-a-real-secret-1234567890","includeReport":false}`
	w := callAPI(app, "/api/chat", payload, "unit-test-token", app.origin)
	if strings.Contains(w.Body.String(), "sk-this-is-not-a-real-secret") {
		t.Fatal("API error echoed secret")
	}
}
func TestTraceAlwaysRequiresSensitiveDataConsent(t *testing.T) {
	app := makeTestServer()
	w := callAPI(app, "/api/trace", `{"confirmSensitiveTrace":false}`, "unit-test-token", app.origin)
	if w.Code != 400 {
		t.Fatalf("trace without consent returned %d", w.Code)
	}
}
func TestJSONRejectsUnexpectedFields(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"deep":false,"runAsSystem":true}`))
	var v struct {
		Deep bool `json:"deep"`
	}
	if err := decodeJSON(req, &v, 1024); err == nil {
		t.Fatal("unexpected JSON parameters accepted")
	}
}
func TestAnalyzeLocallyWithoutInternet(t *testing.T) {
	app := makeTestServer()
	app.latest = &Report{Assessment: []string{"Keep the existing config"}}
	w := callAPI(app, "/api/local", `{"question":"How can I optimize audio?"}`, "unit-test-token", app.origin)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Audio:") {
		t.Fatalf("local advice failed: %d %s", w.Code, w.Body.String())
	}
}
func TestResponseParser(t *testing.T) {
	result, err := parseResponse([]byte(`{"output":[{"type":"reasoning"},{"type":"message","content":[{"type":"output_text","text":"Tested output"}]}]}`))
	if err != nil || result != "Tested output" {
		t.Fatal(result, err)
	}
}
func TestClientIsNotCalledBeforeValidation(t *testing.T) {
	app := makeTestServer()
	w := callAPI(app, "/api/chat", `{"question":"x","apiKey":"not-a-key"}`, "unit-test-token", app.origin)
	if w.Code != 400 {
		t.Fatal("bad key must be rejected")
	}
}
func TestAIUsesStoreFalseAndOptionalReport(t *testing.T) {
	var body map[string]any
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://api.openai.com/v1/responses" {
			t.Fatal("wrong API domain")
		}
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{"output":[{"content":[{"type":"output_text","text":"Safe diagnosis"}]}]}`)), Header: http.Header{}}, nil
	})
	c := &http.Client{Transport: transport}
	input := chatInput{Question: "Help with latency", ApiKey: "sk-example-personal-key-not-a-real-key"}
	ans, err := askAI(context.Background(), c, input, &Report{WindowsData: map[string]any{"cpu": "Private CPU"}})
	if err != nil || ans != "Safe diagnosis" {
		t.Fatal(ans, err)
	}
	if body["store"] != false {
		t.Fatal("API retention was not disabled")
	}
	serialized, _ := json.Marshal(body)
	if strings.Contains(string(serialized), "Private CPU") {
		t.Fatal("report was sent without consent")
	}
	input.IncludeReport = true
	_, err = askAI(context.Background(), c, input, &Report{WindowsData: map[string]any{"cpu": "Private CPU"}})
	if err != nil {
		t.Fatal(err)
	}
	serialized, _ = json.Marshal(body)
	if !strings.Contains(string(serialized), "Private CPU") {
		t.Fatal("explicitly consented report not included")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestHeavyTasksDoNotOverlap(t *testing.T) {
	app := makeTestServer()
	app.workMu.Lock()
	defer app.workMu.Unlock()
	scan := callAPI(app, "/api/scan", `{"deep":false}`, "unit-test-token", app.origin)
	if scan.Code != 409 {
		t.Fatalf("scan started during other heavy task: %d", scan.Code)
	}
	trace := callAPI(app, "/api/trace", `{"confirmSensitiveTrace":true}`, "unit-test-token", app.origin)
	if trace.Code != 409 {
		t.Fatalf("kernel tracing started during other heavy task: %d", trace.Code)
	}
}
