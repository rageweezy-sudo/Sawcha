package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testApp(t *testing.T) (*App, *httptest.Server, *http.Client) {
	t.Helper()
	a := &App{token: "secret-key", root: t.TempDir(), hardware: map[string]interface{}{}, games: []Game{}, advice: []Advice{}, config: SavedConfig{Preset: "Stock"}, live: map[string]interface{}{}}
	a.initAuth()
	server := httptest.NewServer(a.routes())
	jar, _ := cookiejar.New(nil)
	return a, server, &http.Client{Jar: jar}
}
func callAuth(t *testing.T, c *http.Client, url, path string, data interface{}) *http.Response {
	t.Helper()
	body, _ := json.Marshal(data)
	req, _ := http.NewRequest("POST", url+path, bytes.NewReader(body))
	req.Header.Set("X-Sawcha-Token", "secret-key")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}
func TestDeveloperGateAndCredentials(t *testing.T) {
	a, srv, client := testApp(t)
	defer srv.Close()
	resp := callAuth(t, client, srv.URL, "/api/developer/report", map[string]string{})
	if resp.StatusCode != 401 {
		t.Fatalf("expected sign-in gate got %s", resp.Status)
	}
	resp.Body.Close()
	resp = callAuth(t, client, srv.URL, "/api/auth/guest", map[string]string{})
	if resp.StatusCode != 200 {
		t.Fatalf("guest: %s", resp.Status)
	}
	resp.Body.Close()
	resp = callAuth(t, client, srv.URL, "/api/developer/report", map[string]string{})
	if resp.StatusCode != 403 {
		t.Fatalf("expected developer gate got %s", resp.Status)
	}
	resp.Body.Close()
	resp = callAuth(t, client, srv.URL, "/api/auth/developer/setup", map[string]string{"password": "correct and long secret"})
	if resp.StatusCode != 200 {
		t.Fatalf("setup: %s", resp.Status)
	}
	resp.Body.Close()
	raw, e := os.ReadFile(filepath.Join(a.root, "developer.json"))
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(raw, []byte("correct and long secret")) {
		t.Fatal("plaintext password stored")
	}
	resp = callAuth(t, client, srv.URL, "/api/auth/developer/unlock", map[string]string{"password": "bad password"})
	if resp.StatusCode != 403 {
		t.Fatalf("expected wrong password rejected; got %s", resp.Status)
	}
	resp.Body.Close()
	resp = callAuth(t, client, srv.URL, "/api/auth/developer/unlock", map[string]string{"password": "correct and long secret"})
	if resp.StatusCode != 200 {
		t.Fatalf("unlock: %s", resp.Status)
	}
	resp.Body.Close()
	resp = callAuth(t, client, srv.URL, "/api/developer/report", map[string]string{})
	if resp.StatusCode != 200 {
		t.Fatalf("dev access: %s", resp.Status)
	}
	resp.Body.Close()
	resp = callAuth(t, client, srv.URL, "/api/auth/developer/lock", map[string]string{})
	if resp.StatusCode != 200 {
		t.Fatalf("lock: %s", resp.Status)
	}
	resp.Body.Close()
	resp = callAuth(t, client, srv.URL, "/api/developer/report", map[string]string{})
	if resp.StatusCode != 403 {
		t.Fatalf("developer gate not restored: %s", resp.Status)
	}
	resp.Body.Close()
	resp = callAuth(t, client, srv.URL, "/api/auth/logout", map[string]string{})
	if resp.StatusCode != 200 {
		t.Fatalf("logout: %s", resp.Status)
	}
	resp.Body.Close()
	resp = callAuth(t, client, srv.URL, "/api/state", map[string]string{})
	if resp.StatusCode != 401 {
		t.Fatalf("logout session survived: %s", resp.Status)
	}
	resp.Body.Close()
}
func TestGoogleConfigurationAndPKCE(t *testing.T) {
	a, srv, client := testApp(t)
	defer srv.Close()
	resp := callAuth(t, client, srv.URL, "/api/auth/google/configure", map[string]string{"clientID": "bad"})
	if resp.StatusCode != 400 {
		t.Fatalf("invalid ID accepted: %s", resp.Status)
	}
	resp.Body.Close()
	valid := "12345-examplekey.apps.googleusercontent.com"
	resp = callAuth(t, client, srv.URL, "/api/auth/google/configure", map[string]string{"clientID": valid})
	if resp.StatusCode != 200 {
		t.Fatalf("config: %s", resp.Status)
	}
	resp.Body.Close()
	b, e := os.ReadFile(filepath.Join(a.root, "oauth.json"))
	if e != nil || !strings.Contains(string(b), valid) {
		t.Fatalf("oauth not saved: %v", e)
	}
	resp = callAuth(t, client, srv.URL, "/api/auth/google/start", map[string]string{})
	if resp.StatusCode != 200 {
		t.Fatalf("start: %s", resp.Status)
	}
	var out map[string]string
	if e = json.NewDecoder(resp.Body).Decode(&out); e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if !strings.Contains(out["url"], "code_challenge_method=S256") || !strings.Contains(out["url"], "scope=openid+email+profile") || out["flowID"] == "" {
		t.Fatalf("invalid oauth URL: %+v", out)
	}
	a.auth.mu.Lock()
	pending := a.auth.pending[out["flowID"]]
	a.auth.mu.Unlock()
	if pending == nil || len(pending.Verifier) < 43 || pending.Nonce == "" {
		t.Fatal("missing PKCE state")
	}
	resp = callAuth(t, client, srv.URL, "/api/auth/google/finish", map[string]string{"flowID": out["flowID"]})
	if resp.StatusCode != 409 {
		t.Fatalf("finished without token: %s", resp.Status)
	}
	resp.Body.Close()
}
func TestUntrustedGoogleJWT(t *testing.T) {
	// No network request should happen if audience/nonce/issuer already fail.
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","kid":"x"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"https://accounts.google.com","aud":"client","nonce":"n","exp":9999999999,"email_verified":true}`))
	_, err := verifyGoogleJWT(context.Background(), header+"."+payload+".fake", "client", "n")
	if err == nil {
		t.Fatal("unsigned identity token accepted")
	}
}
