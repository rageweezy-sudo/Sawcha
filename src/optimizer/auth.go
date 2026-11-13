package main

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const sessionCookie = "sawcha_session"
const sessionDuration = 12 * time.Hour

type LoginIdentity struct {
	Name     string `json:"name"`
	Email    string `json:"email,omitempty"`
	Provider string `json:"provider"`
	Role     string `json:"role"`
}
type LoginSession struct {
	Identity LoginIdentity
	Expires  time.Time
}
type OAuthPending struct {
	ClientID string
	Verifier string
	Nonce    string
	Expires  time.Time
	Identity *LoginIdentity
	Failure  string
}
type DevCredential struct {
	Salt       string `json:"salt"`
	Hash       string `json:"hash"`
	Iterations int    `json:"iterations"`
}
type OAuthConfiguration struct {
	ClientID string `json:"clientID"`
}
type authEngine struct {
	mu             sync.Mutex
	sessions       map[string]LoginSession
	pending        map[string]*OAuthPending
	googleClientID string
	developer      DevCredential
	failures       int
	nextAttempt    time.Time
}

func randURL(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func (a *App) initAuth() {
	a.auth.sessions = map[string]LoginSession{}
	a.auth.pending = map[string]*OAuthPending{}
	var oauth OAuthConfiguration
	if b, e := os.ReadFile(filepath.Join(a.root, "oauth.json")); e == nil {
		_ = json.Unmarshal(b, &oauth)
	}
	a.auth.googleClientID = strings.TrimSpace(oauth.ClientID)
	if b, e := os.ReadFile(filepath.Join(a.root, "developer.json")); e == nil {
		_ = json.Unmarshal(b, &a.auth.developer)
	}
}
func (a *App) loginIdentity(r *http.Request) (LoginIdentity, bool) {
	c, e := r.Cookie(sessionCookie)
	if e != nil || len(c.Value) < 32 {
		return LoginIdentity{}, false
	}
	a.auth.mu.Lock()
	defer a.auth.mu.Unlock()
	s, ok := a.auth.sessions[c.Value]
	if !ok || time.Now().After(s.Expires) {
		delete(a.auth.sessions, c.Value)
		return LoginIdentity{}, false
	}
	return s.Identity, true
}
func (a *App) createSession(w http.ResponseWriter, id LoginIdentity) {
	key := randURL(40)
	a.auth.mu.Lock()
	a.auth.sessions[key] = LoginSession{Identity: id, Expires: time.Now().Add(sessionDuration)}
	a.auth.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: key, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: int(sessionDuration.Seconds())})
}
func (a *App) elevateSession(r *http.Request, role string) (LoginIdentity, error) {
	cookie, e := r.Cookie(sessionCookie)
	if e != nil {
		return LoginIdentity{}, errors.New("Sign in first")
	}
	a.auth.mu.Lock()
	defer a.auth.mu.Unlock()
	current, ok := a.auth.sessions[cookie.Value]
	if !ok || time.Now().After(current.Expires) {
		return LoginIdentity{}, errors.New("Session expired")
	}
	current.Identity.Role = role
	a.auth.sessions[cookie.Value] = current
	return current.Identity, nil
}
func (a *App) clearSession(w http.ResponseWriter, r *http.Request) {
	c, e := r.Cookie(sessionCookie)
	if e == nil {
		a.auth.mu.Lock()
		delete(a.auth.sessions, c.Value)
		a.auth.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}
func (a *App) authStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := a.loginIdentity(r)
	a.auth.mu.Lock()
	google := a.auth.googleClientID != ""
	dev := a.auth.developer.Hash != ""
	a.auth.mu.Unlock()
	send(w, map[string]interface{}{"signedIn": ok, "identity": id, "googleConfigured": google, "developerConfigured": dev, "version": version})
}
func (a *App) authGuest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		bad(w, "POST required", 405)
		return
	}
	a.createSession(w, LoginIdentity{Name: "Local guest", Provider: "offline", Role: "user"})
	send(w, map[string]interface{}{"signedIn": true, "identity": LoginIdentity{Name: "Local guest", Provider: "offline", Role: "user"}})
}
func (a *App) authLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		bad(w, "POST required", 405)
		return
	}
	a.clearSession(w, r)
	send(w, map[string]bool{"ok": true})
}

var googleDesktopID = regexp.MustCompile(`^[0-9]+-[a-zA-Z0-9_\-]+\.apps\.googleusercontent\.com$`)

func (a *App) googleConfigure(w http.ResponseWriter, r *http.Request) {
	var req OAuthConfiguration
	if e := decode(r, &req); e != nil {
		bad(w, e.Error(), 400)
		return
	}
	req.ClientID = strings.TrimSpace(req.ClientID)
	if req.ClientID != "" && !googleDesktopID.MatchString(req.ClientID) {
		bad(w, "Enter a valid Google OAuth Desktop client ID ending in .apps.googleusercontent.com", 400)
		return
	}
	if e := writeJSON(filepath.Join(a.root, "oauth.json"), req); e != nil {
		bad(w, "Could not save OAuth client ID: "+e.Error(), 500)
		return
	}
	a.auth.mu.Lock()
	a.auth.googleClientID = req.ClientID
	a.auth.mu.Unlock()
	send(w, map[string]interface{}{"ok": true, "configured": req.ClientID != ""})
}
func (a *App) googleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		bad(w, "POST required", 405)
		return
	}
	a.auth.mu.Lock()
	id := a.auth.googleClientID
	a.auth.mu.Unlock()
	if !googleDesktopID.MatchString(id) {
		bad(w, "Google login isn't configured. Add a Desktop OAuth client ID first.", 400)
		return
	}
	state := randURL(32)
	nonce := randURL(32)
	verifier := randURL(48)
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])
	a.auth.mu.Lock()
	for k, v := range a.auth.pending {
		if time.Now().After(v.Expires) {
			delete(a.auth.pending, k)
		}
	}
	a.auth.pending[state] = &OAuthPending{ClientID: id, Verifier: verifier, Nonce: nonce, Expires: time.Now().Add(6 * time.Minute)}
	a.auth.mu.Unlock()
	redirect := fmt.Sprintf("http://127.0.0.1:%d/auth/google/callback", a.port)
	args := url.Values{"client_id": {id}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {"openid email profile"}, "state": {state}, "nonce": {nonce}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}, "prompt": {"select_account"}}
	send(w, map[string]string{"url": "https://accounts.google.com/o/oauth2/v2/auth?" + args.Encode(), "flowID": state})
}
func (a *App) googleCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	flowID := r.URL.Query().Get("state")
	a.auth.mu.Lock()
	p, exists := a.auth.pending[flowID]
	if !exists || time.Now().After(p.Expires) {
		a.auth.mu.Unlock()
		io.WriteString(w, "<h2>Invalid or expired sign-in. Return to Sawcha Optimizer and retry.</h2>")
		return
	}
	if p.Identity != nil || p.Failure != "" {
		a.auth.mu.Unlock()
		io.WriteString(w, "<h2>This sign-in has already finished.</h2>")
		return
	}
	client, verifier, nonce := p.ClientID, p.Verifier, p.Nonce
	a.auth.mu.Unlock()
	errText := r.URL.Query().Get("error")
	var identity LoginIdentity
	var err error
	if errText != "" {
		err = fmt.Errorf("Google sign-in was cancelled or refused: %s", errText)
	} else {
		code := r.URL.Query().Get("code")
		if len(code) < 8 {
			err = errors.New("Google did not return an authorization code")
		} else {
			ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
			defer cancel()
			identity, err = googleExchange(ctx, client, verifier, nonce, code, fmt.Sprintf("http://127.0.0.1:%d/auth/google/callback", a.port))
		}
	}
	a.auth.mu.Lock()
	if p2, ok := a.auth.pending[flowID]; ok && p2 == p {
		p.Verifier = "" // Don't retain a verifier after use
		if err != nil {
			p.Failure = err.Error()
		} else {
			p.Identity = &identity
		}
	}
	a.auth.mu.Unlock()
	if err != nil {
		io.WriteString(w, "<!doctype html><html><body style='font:16px Arial;background:#333;color:#ff666d;padding:50px'><h2>Google login could not be completed.</h2><p>Return to SAWCHA // OPTIMIZER and retry.</p></body></html>")
		return
	}
	io.WriteString(w, "<!doctype html><html><body style='font:16px Arial;background:#333;color:#ff666d;padding:50px'><h1>SAWCHA // OPTIMIZER</h1><h2>Google sign-in completed.</h2><p>Return to the Optimizer window. You can close this tab.</p></body></html>")
}
func (a *App) googleStatus(w http.ResponseWriter, r *http.Request) {
	var q struct {
		FlowID string `json:"flowID"`
	}
	if e := decode(r, &q); e != nil {
		bad(w, e.Error(), 400)
		return
	}
	a.auth.mu.Lock()
	p, ok := a.auth.pending[q.FlowID]
	if !ok || time.Now().After(p.Expires) {
		a.auth.mu.Unlock()
		bad(w, "Sign-in expired; retry", 410)
		return
	}
	failure, done := p.Failure, p.Identity != nil
	a.auth.mu.Unlock()
	send(w, map[string]interface{}{"done": done, "error": failure})
}
func (a *App) googleFinish(w http.ResponseWriter, r *http.Request) {
	var q struct {
		FlowID string `json:"flowID"`
	}
	if e := decode(r, &q); e != nil {
		bad(w, e.Error(), 400)
		return
	}
	a.auth.mu.Lock()
	p, ok := a.auth.pending[q.FlowID]
	if !ok || time.Now().After(p.Expires) {
		a.auth.mu.Unlock()
		bad(w, "Login expired", 410)
		return
	}
	if p.Identity == nil {
		a.auth.mu.Unlock()
		bad(w, "Google login is not completed", 409)
		return
	}
	identity := *p.Identity
	delete(a.auth.pending, q.FlowID) // one-time flow code
	a.auth.mu.Unlock()
	a.createSession(w, identity)
	send(w, map[string]interface{}{"ok": true, "identity": identity})
}
func deriveDevKey(password string, salt []byte, iterations int) []byte {
	// PBKDF2-HMAC-SHA256, 32 bytes: do not store plaintext passwords.
	u := make([]byte, len(salt)+4)
	copy(u, salt)
	binary.BigEndian.PutUint32(u[len(salt):], 1)
	mac := hmac.New(sha256.New, []byte(password))
	mac.Write(u)
	t := mac.Sum(nil)
	result := append([]byte(nil), t...)
	for i := 1; i < iterations; i++ {
		mac = hmac.New(sha256.New, []byte(password))
		mac.Write(t)
		t = mac.Sum(nil)
		for j := range result {
			result[j] ^= t[j]
		}
	}
	return result
}
func (a *App) developerSetup(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.loginIdentity(r); !ok {
		bad(w, "Sign in first", 401)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if e := decode(r, &req); e != nil {
		bad(w, e.Error(), 400)
		return
	}
	if len([]rune(req.Password)) < 12 || len(req.Password) > 256 {
		bad(w, "Use a developer passphrase between 12 and 256 characters", 400)
		return
	}
	a.auth.mu.Lock()
	if a.auth.developer.Hash != "" {
		a.auth.mu.Unlock()
		bad(w, "Developer passphrase already configured", 409)
		return
	}
	a.auth.mu.Unlock()
	salt := make([]byte, 24)
	if _, e := rand.Read(salt); e != nil {
		bad(w, "Secure random generator unavailable", 500)
		return
	}
	credential := DevCredential{Salt: base64.RawStdEncoding.EncodeToString(salt), Hash: base64.RawStdEncoding.EncodeToString(deriveDevKey(req.Password, salt, 180000)), Iterations: 180000}
	a.auth.mu.Lock()
	defer a.auth.mu.Unlock()
	if a.auth.developer.Hash != "" {
		bad(w, "Developer setup already completed", 409)
		return
	}
	if e := writeJSON(filepath.Join(a.root, "developer.json"), credential); e != nil {
		bad(w, "Could not save developer credentials: "+e.Error(), 500)
		return
	}
	a.auth.developer = credential
	// Session role is changed after releasing auth lock, below.
	send(w, map[string]bool{"ok": true})
}
func (a *App) developerUnlock(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.loginIdentity(r); !ok {
		bad(w, "Sign in first", 401)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if e := decode(r, &req); e != nil {
		bad(w, e.Error(), 400)
		return
	}
	a.auth.mu.Lock()
	c := a.auth.developer
	if c.Hash == "" {
		a.auth.mu.Unlock()
		bad(w, "Set up developer access first", 400)
		return
	}
	if time.Now().Before(a.auth.nextAttempt) {
		a.auth.mu.Unlock()
		bad(w, "Too many attempts. Try again shortly.", 429)
		return
	}
	a.auth.mu.Unlock()
	salt, se := base64.RawStdEncoding.DecodeString(c.Salt)
	want, we := base64.RawStdEncoding.DecodeString(c.Hash)
	if se != nil || we != nil || c.Iterations < 50000 || c.Iterations > 1000000 {
		bad(w, "Developer credential file is invalid", 500)
		return
	}
	actual := deriveDevKey(req.Password, salt, c.Iterations)
	if subtle.ConstantTimeCompare(actual, want) != 1 {
		a.auth.mu.Lock()
		a.auth.failures++
		if a.auth.failures >= 5 {
			a.auth.nextAttempt = time.Now().Add(30 * time.Second)
			a.auth.failures = 0
		}
		a.auth.mu.Unlock()
		bad(w, "Incorrect developer passphrase", 403)
		return
	}
	a.auth.mu.Lock()
	a.auth.failures = 0
	a.auth.nextAttempt = time.Time{}
	a.auth.mu.Unlock()
	id, e := a.elevateSession(r, "developer")
	if e != nil {
		bad(w, e.Error(), 401)
		return
	}
	send(w, map[string]interface{}{"ok": true, "identity": id})
}
func (a *App) developerLock(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		bad(w, "POST required", 405)
		return
	}
	if _, e := a.elevateSession(r, "user"); e != nil {
		bad(w, e.Error(), 401)
		return
	}
	send(w, map[string]bool{"ok": true})
}
func (a *App) developerReport(w http.ResponseWriter, r *http.Request) {
	id, ok := a.loginIdentity(r)
	if !ok || id.Role != "developer" {
		bad(w, "Developer access required", 403)
		return
	}
	a.mu.RLock()
	report := map[string]interface{}{"version": version, "system": a.hardware, "games": a.games, "advice": a.advice, "recentJobs": a.jobs, "benchmarkCount": len(a.history), "logs": a.logEntries, "scanStage": a.stage, "scanReady": a.ready, "lastScan": a.scanned, "platform": osName(), "sessionProvider": id.Provider}
	a.mu.RUnlock()
	send(w, report) // Explicit: no OAuth ID token, client ID, password, cookies or API key
}
func osName() string { return "Windows desktop (or cross-platform test build)" }

func googleExchange(ctx context.Context, clientID, verifier, nonce, code, redirect string) (LoginIdentity, error) {
	form := url.Values{"code": {code}, "client_id": {clientID}, "code_verifier": {verifier}, "redirect_uri": {redirect}, "grant_type": {"authorization_code"}}
	req, e := http.NewRequestWithContext(ctx, "POST", "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	if e != nil {
		return LoginIdentity{}, e
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, e := http.DefaultClient.Do(req)
	if e != nil {
		return LoginIdentity{}, fmt.Errorf("Google token exchange failed: %w", e)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return LoginIdentity{}, fmt.Errorf("Google returned HTTP %d for token exchange", response.StatusCode)
	}
	var result struct {
		IDToken string `json:"id_token"`
	}
	if e := json.NewDecoder(io.LimitReader(response.Body, 100000)).Decode(&result); e != nil {
		return LoginIdentity{}, e
	}
	return verifyGoogleJWT(ctx, result.IDToken, clientID, nonce)
}
func verifyGoogleJWT(ctx context.Context, token, clientID, nonce string) (LoginIdentity, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return LoginIdentity{}, errors.New("Invalid Google ID token format")
	}
	head, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return LoginIdentity{}, err
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if json.Unmarshal(head, &header) != nil || header.Alg != "RS256" || header.Kid == "" {
		return LoginIdentity{}, errors.New("Unsupported Google token signing algorithm")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return LoginIdentity{}, err
	}
	var claims struct {
		Iss           string `json:"iss"`
		Aud           string `json:"aud"`
		Sub           string `json:"sub"`
		Exp           int64  `json:"exp"`
		Iat           int64  `json:"iat"`
		Nonce         string `json:"nonce"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err = json.Unmarshal(payload, &claims); err != nil {
		return LoginIdentity{}, err
	}
	if (claims.Iss != "accounts.google.com" && claims.Iss != "https://accounts.google.com") || claims.Aud != clientID || claims.Sub == "" || claims.Nonce != nonce || !claims.EmailVerified || claims.Exp <= time.Now().Unix() || claims.Iat > time.Now().Add(2*time.Minute).Unix() {
		return LoginIdentity{}, errors.New("Google identity token validation failed")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.googleapis.com/oauth2/v3/certs", nil)
	if err != nil {
		return LoginIdentity{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return LoginIdentity{}, fmt.Errorf("Could not download Google's public keys: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return LoginIdentity{}, fmt.Errorf("Google public key server returned HTTP %d", resp.StatusCode)
	}
	var keys struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			Alg string `json:"alg"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 200000)).Decode(&keys); err != nil {
		return LoginIdentity{}, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return LoginIdentity{}, err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	for _, k := range keys.Keys {
		if k.Kid != header.Kid || k.Kty != "RSA" || k.Alg != "RS256" {
			continue
		}
		n, ne := base64.RawURLEncoding.DecodeString(k.N)
		e, ee := base64.RawURLEncoding.DecodeString(k.E)
		if ne != nil || ee != nil || len(e) > 4 || len(e) == 0 {
			continue
		}
		exp := 0
		for _, v := range e {
			exp = exp*256 + int(v)
		}
		if exp < 3 || exp%2 == 0 {
			continue
		}
		pub := rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exp}
		if pub.N.BitLen() < 2048 {
			continue
		}
		if rsa.VerifyPKCS1v15(&pub, crypto.SHA256, digest[:], signature) != nil {
			return LoginIdentity{}, errors.New("Invalid Google ID token signature")
		}
		displayName := claims.Name
		if strings.TrimSpace(displayName) == "" {
			displayName = claims.Email
		}
		return LoginIdentity{Name: displayName, Email: claims.Email, Provider: "google", Role: "user"}, nil
	}
	return LoginIdentity{}, errors.New("Google signing key unavailable; please retry")
}
func developerVerifyData(a *App) map[string]interface{} {
	// Display only; no secrets returned to frontend.
	a.auth.mu.Lock()
	defer a.auth.mu.Unlock()
	return map[string]interface{}{"googleConfigured": a.auth.googleClientID != "", "developerConfigured": a.auth.developer.Hash != "", "sessionCount": len(a.auth.sessions), "pendingGoogleLogins": len(a.auth.pending), "now": strconv.FormatInt(time.Now().Unix(), 10)}
}
