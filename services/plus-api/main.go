// SAWCHA Plus: customer-only subscription/licensing API.
// NO developer routes, passwords, private vault source, or hard-coded Stripe secrets.
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var validDevice = regexp.MustCompile(`^[a-f0-9]{64}$`)
var validEmail = regexp.MustCompile(`^[^\s@]{1,64}@[^\s@]{1,190}$`)

type config struct {
	BaseURL, StripeKey, PriceID, WebhookKey, DataDir string
	DataKey                                          []byte
	GiftPublicKey                                    ed25519.PublicKey
}
type purchase struct {
	ID             string `json:"id"`
	Email          string `json:"email"`
	DeviceHash     string `json:"device_hash"`
	ClaimHash      string `json:"claim_hash"`
	SessionID      string `json:"session_id"`
	SubscriptionID string `json:"subscription_id"`
	CustomerID     string `json:"customer_id"`
	LicenseHash    string `json:"license_hash"`
	LicenseEnc     string `json:"license_enc"`
	LicenseLast4   string `json:"license_last4"`
	Paid           bool   `json:"paid"`
	Active         bool   `json:"active"`
	Expires        int64  `json:"expires"`
	Created        int64  `json:"created"`
}
type database struct {
	Purchases    map[string]*purchase `json:"purchases"`
	Events       map[string]int64     `json:"events"`
	Gifts        map[string]*giftUse  `json:"gifts"`
	RevokedGifts map[string]int64     `json:"revoked_gifts"`
}
type service struct {
	mu         sync.Mutex
	db         database
	cfg        config
	httpClient *http.Client
	stripeAPI  string // private test seam; production always defaults to Stripe.
	path       string
	throttleMu sync.Mutex
	throttle   map[string][]time.Time
}

func token(n int) string {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func mac(key []byte, domain, value string) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(domain + ":" + value))
	return hex.EncodeToString(h.Sum(nil))
}
func same(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
func (s *service) encrypt(v string) (string, error) {
	block, e := aes.NewCipher(s.cfg.DataKey)
	if e != nil {
		return "", e
	}
	a, e := cipher.NewGCM(block)
	if e != nil {
		return "", e
	}
	nonce := make([]byte, a.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return "", e
	}
	return hex.EncodeToString(a.Seal(nonce, nonce, []byte(v), []byte("sawcha-plus-license-v1"))), nil
}
func (s *service) decrypt(v string) (string, error) {
	raw, e := hex.DecodeString(v)
	if e != nil {
		return "", e
	}
	block, e := aes.NewCipher(s.cfg.DataKey)
	if e != nil {
		return "", e
	}
	a, e := cipher.NewGCM(block)
	if e != nil {
		return "", e
	}
	if len(raw) < a.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	plain, e := a.Open(nil, raw[:a.NonceSize()], raw[a.NonceSize():], []byte("sawcha-plus-license-v1"))
	return string(plain), e
}
func newService(cfg config) (*service, error) {
	if len(cfg.DataKey) != 32 {
		return nil, errors.New("SAWCHA_DATA_KEY must be 32 random bytes, hex encoded")
	}
	if cfg.DataDir == "" {
		return nil, errors.New("SAWCHA_DATA_DIR required (use a persistent private volume)")
	}
	if e := os.MkdirAll(cfg.DataDir, 0700); e != nil {
		return nil, e
	}
	s := &service{cfg: cfg, db: database{Purchases: map[string]*purchase{}, Events: map[string]int64{}}, httpClient: &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, stripeAPI: "https://api.stripe.com", throttle: map[string][]time.Time{}, path: filepath.Join(cfg.DataDir, "plus.json")}
	if b, e := os.ReadFile(s.path); e == nil {
		if e = json.Unmarshal(b, &s.db); e != nil {
			return nil, fmt.Errorf("cannot read license database: %w", e)
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return nil, e
	}
	if s.db.Purchases == nil {
		s.db.Purchases = map[string]*purchase{}
	}
	if s.db.Events == nil {
		s.db.Events = map[string]int64{}
	}
	if s.db.Gifts == nil {
		s.db.Gifts = map[string]*giftUse{}
	}
	if s.db.RevokedGifts == nil {
		s.db.RevokedGifts = map[string]int64{}
	}
	return s, nil
}
func (s *service) persist() error {
	b, e := json.Marshal(s.db)
	if e != nil {
		return e
	}
	p := s.path + ".tmp"
	if e = os.WriteFile(p, b, 0600); e != nil {
		return e
	}
	return os.Rename(p, s.path)
}
func jsonReply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func errReply(w http.ResponseWriter, code int, msg string) {
	jsonReply(w, code, map[string]string{"error": msg})
}
func body(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Method != "POST" {
		errReply(w, 405, "POST required")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		errReply(w, 400, "invalid JSON request")
		return false
	}
	return true
}
func (s *service) limited(r *http.Request) bool {
	// Deployment-specific trusted proxy handling is intentionally omitted: a forwarded IP is spoofable.
	// Railway's proxy may group clients under one peer, so throttle conservatively.
	k := r.RemoteAddr
	if host, _, e := net.SplitHostPort(k); e == nil {
		k = host
	} // conservative peer grouping
	s.throttleMu.Lock()
	defer s.throttleMu.Unlock()
	now := time.Now()
	q := s.throttle[k][:0]
	for _, t := range s.throttle[k] {
		if now.Sub(t) < time.Minute {
			q = append(q, t)
		}
	}
	if len(q) > 25 {
		return false
	}
	q = append(q, now)
	s.throttle[k] = q
	if len(s.throttle) > 2000 {
		s.throttle = map[string][]time.Time{}
	}
	return true
}
func (s *service) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		jsonReply(w, 200, map[string]any{"service": "sawcha-plus", "checkout_configured": s.checkoutReady()})
	})
	mux.HandleFunc("GET /", s.home)
	mux.HandleFunc("GET /complete", s.complete)
	mux.HandleFunc("GET /cancel", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "Checkout canceled. Sawcha Basic remains free. Return to the app.")
	})
	mux.HandleFunc("POST /v1/checkout", s.checkout)
	mux.HandleFunc("POST /v1/checkout/status", s.status)
	mux.HandleFunc("POST /v1/activate", s.activate)
	mux.HandleFunc("POST /v1/gift/redeem", s.redeemGift)
	mux.HandleFunc("POST /v1/gift/revoke", s.revokeGift)
	mux.HandleFunc("POST /v1/verify", s.verify)
	mux.HandleFunc("POST /v1/portal", s.portal)
	mux.HandleFunc("POST /stripe/webhook", s.webhook)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; form-action 'self'")
		if r.URL.Path != "/stripe/webhook" && r.URL.Path != "/health" && !s.limited(r) {
			errReply(w, 429, "slow down and try again later")
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func (s *service) checkoutReady() bool {
	return (strings.HasPrefix(s.cfg.StripeKey, "sk_test_") || (os.Getenv("SAWCHA_ENABLE_LIVE_BILLING") == "I_ACKNOWLEDGE_LIVE_CHARGES" && strings.HasPrefix(s.cfg.StripeKey, "sk_live_"))) && strings.HasPrefix(s.cfg.PriceID, "price_") && len(s.cfg.WebhookKey) > 10 && strings.HasPrefix(s.cfg.BaseURL, "https://")
}
func (s *service) home(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, `<!doctype html><html lang="en"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Sawcha Plus</title><style>body{background:#33343b;color:#f4f4f8;font:18px system-ui;max-width:760px;margin:10vh auto;padding:26px}h1,strong{color:#ff4855}.card{background:#41424a;border-radius:16px;padding:24px}small{color:#b4b4bb}</style><h1>SAWCHA // PLUS</h1><div class="card"><h2>$1.99 / month</h2><p>Optional Plus features for Sawcha Optimizer on Windows 10 and 11.</p><p>To purchase, open the installed Sawcha app and choose <strong>Plus Membership → Subscribe</strong>.</p><small>Basic remains free. Payment is handled by Stripe Checkout. You can manage or cancel your subscription from the app.</small></div></html>`)
}
func (s *service) complete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, `<!doctype html><html lang="en"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Sawcha subscription</title><style>body{background:#33343b;color:white;font:20px system-ui;max-width:700px;margin:12vh auto;padding:20px}h1{color:#ff4855}</style><h1>Payment submitted</h1><p>Return to Sawcha Optimizer and select <b>Check purchase status</b>. Plus activates only after Stripe confirms a paid subscription.</p><p>Keep your payment details private. If payment processing is still pending, try again in the app shortly.</p></html>`)
}
func (s *service) stripeForm(ctx *http.Request, endpoint string, data url.Values) (map[string]any, error) {
	req, e := http.NewRequestWithContext(ctx.Context(), "POST", s.stripeAPI+endpoint, strings.NewReader(data.Encode()))
	if e != nil {
		return nil, e
	}
	req.SetBasicAuth(s.cfg.StripeKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, e := s.httpClient.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if e != nil {
		return nil, e
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Stripe returned %d", resp.StatusCode)
	}
	var out map[string]any
	if e = json.Unmarshal(raw, &out); e != nil {
		return nil, e
	}
	return out, nil
}
func (s *service) stripeGet(ctx *http.Request, endpoint string) (map[string]any, error) {
	req, e := http.NewRequestWithContext(ctx.Context(), "GET", s.stripeAPI+endpoint, nil)
	if e != nil {
		return nil, e
	}
	req.SetBasicAuth(s.cfg.StripeKey, "")
	resp, e := s.httpClient.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(resp.Body, 128<<10))
	if e != nil {
		return nil, e
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Stripe lookup returned %d", resp.StatusCode)
	}
	var out map[string]any
	if e = json.Unmarshal(raw, &out); e != nil {
		return nil, e
	}
	return out, nil
}
func field(m map[string]any, k string) string       { v, _ := m[k].(string); return v }
func obj(m map[string]any, k string) map[string]any { v, _ := m[k].(map[string]any); return v }
func (s *service) checkout(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		DeviceID string `json:"device_id"`
	}
	if !body(w, r, &in) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if !validEmail.MatchString(in.Email) || !validDevice.MatchString(in.DeviceID) {
		errReply(w, 400, "valid email and device ID required")
		return
	}
	if !s.checkoutReady() {
		errReply(w, 503, "subscriptions not configured yet")
		return
	}
	id, claim := token(16), token(32)
	p := &purchase{ID: id, Email: in.Email, ClaimHash: mac(s.cfg.DataKey, "claim", claim), DeviceHash: mac(s.cfg.DataKey, "device", in.DeviceID), Created: time.Now().Unix()}
	s.mu.Lock()
	s.db.Purchases[id] = p
	e := s.persist()
	s.mu.Unlock()
	if e != nil {
		errReply(w, 500, "database unavailable")
		return
	}
	form := url.Values{"mode": {"subscription"}, "customer_email": {in.Email}, "line_items[0][price]": {s.cfg.PriceID}, "line_items[0][quantity]": {"1"}, "client_reference_id": {id}, "metadata[purchase_id]": {id}, "subscription_data[metadata][purchase_id]": {id}, "success_url": {s.cfg.BaseURL + "/complete?session_id={CHECKOUT_SESSION_ID}"}, "cancel_url": {s.cfg.BaseURL + "/cancel"}}
	sess, e := s.stripeForm(r, "/v1/checkout/sessions", form)
	if e != nil {
		errReply(w, 502, "Stripe Checkout temporarily unavailable")
		return
	}
	sid, link := field(sess, "id"), field(sess, "url")
	if !strings.HasPrefix(sid, "cs_") || !strings.HasPrefix(link, "https://checkout.stripe.com/") {
		errReply(w, 502, "unexpected checkout response")
		return
	}
	s.mu.Lock()
	p.SessionID = sid
	e = s.persist()
	s.mu.Unlock()
	if e != nil {
		errReply(w, 500, "database unavailable")
		return
	}
	jsonReply(w, 200, map[string]string{"checkout_url": link, "claim_token": claim})
}
func (s *service) findClaim(token, device string) *purchase {
	if len(token) != 64 || !validDevice.MatchString(device) {
		return nil
	}
	h := mac(s.cfg.DataKey, "claim", token)
	d := mac(s.cfg.DataKey, "device", device)
	for _, p := range s.db.Purchases {
		if same(h, p.ClaimHash) && same(d, p.DeviceHash) {
			return p
		}
	}
	return nil
}
func (s *service) status(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Claim  string `json:"claim_token"`
		Device string `json:"device_id"`
	}
	if !body(w, r, &in) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.findClaim(in.Claim, in.Device)
	if p == nil {
		errReply(w, 404, "purchase not found")
		return
	}
	if !p.Active || !p.Paid || time.Now().Unix() >= p.Expires {
		jsonReply(w, 200, map[string]any{"active": false, "plan": "basic", "message": "Payment pending, canceled, or expired"})
		return
	}
	k, e := s.decrypt(p.LicenseEnc)
	if e != nil {
		errReply(w, 500, "license data unavailable")
		return
	}
	jsonReply(w, 200, map[string]any{"active": true, "plan": "plus", "license_key": k, "expires_at": time.Unix(p.Expires, 0).UTC().Format(time.RFC3339)})
}
func (s *service) result(key, device string, activate bool) map[string]any {
	out := map[string]any{"active": false, "plan": "basic", "features": []string{}, "message": "Basic is available at no cost"}
	if strings.HasPrefix(key, giftPrefix) {
		return s.giftStatus(key, device, false)
	}
	if !validDevice.MatchString(device) || len(key) < 40 || len(key) > 140 || !strings.HasPrefix(key, "SAWCHA-PLUS-") {
		return out
	}
	kh, dh := mac(s.cfg.DataKey, "license", key), mac(s.cfg.DataKey, "device", device)
	for _, p := range s.db.Purchases {
		if !same(p.LicenseHash, kh) {
			continue
		}
		if !p.Paid || !p.Active || time.Now().Unix() >= p.Expires {
			out["message"] = "Plus subscription inactive"
			return out
		}
		if p.DeviceHash != dh {
			out["message"] = "License belongs to another installation"
			return out
		}
		out["active"] = true
		out["plan"] = "plus"
		out["features"] = []string{"game_profiles_advanced", "frame_analysis", "plus_guidance"}
		out["expires_at"] = time.Unix(p.Expires, 0).UTC().Format(time.RFC3339)
		out["message"] = "Plus subscription verified"
		return out
	}
	return out
}
func (s *service) activate(w http.ResponseWriter, r *http.Request) { s.verify(w, r) }
func (s *service) verify(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key      string `json:"key"`
		DeviceID string `json:"device_id"`
	}
	if !body(w, r, &in) {
		return
	}
	s.mu.Lock()
	out := s.result(strings.TrimSpace(in.Key), in.DeviceID, false)
	s.mu.Unlock()
	jsonReply(w, 200, out)
}
func (s *service) portal(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key      string `json:"key"`
		DeviceID string `json:"device_id"`
	}
	if !body(w, r, &in) {
		return
	}
	s.mu.Lock()
	customer := ""
	hash := mac(s.cfg.DataKey, "license", in.Key)
	deviceHash := mac(s.cfg.DataKey, "device", in.DeviceID)
	if validDevice.MatchString(in.DeviceID) {
		for _, p := range s.db.Purchases {
			if p.LicenseHash != "" && same(p.LicenseHash, hash) && same(p.DeviceHash, deviceHash) {
				customer = p.CustomerID
				break
			}
		}
	}
	s.mu.Unlock()
	if customer == "" {
		errReply(w, 403, "valid Plus key and original installation required")
		return
	}
	form := url.Values{"customer": {customer}, "return_url": {s.cfg.BaseURL + "/"}}
	out, e := s.stripeForm(r, "/v1/billing_portal/sessions", form)
	if e != nil {
		errReply(w, 502, "subscription portal unavailable")
		return
	}
	link := field(out, "url")
	if !strings.HasPrefix(link, "https://billing.stripe.com/") {
		errReply(w, 502, "unexpected billing portal response")
		return
	}
	jsonReply(w, 200, map[string]string{"url": link})
}

// Stripe signs the raw request bytes; never trust an event payload before verification.
func verifySignature(secret, header string, payload []byte, now time.Time) bool {
	var stamp int64
	var signatures []string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			fmt.Sscan(v, &stamp)
		case "v1":
			signatures = append(signatures, v)
		}
	}
	if stamp <= 0 || now.Unix()-stamp > 300 || stamp-now.Unix() > 300 {
		return false
	}
	macer := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(macer, "%d.", stamp)
	macer.Write(payload)
	expected := macer.Sum(nil)
	for _, v := range signatures {
		got, e := hex.DecodeString(v)
		if e == nil && hmac.Equal(expected, got) {
			return true
		}
	}
	return false
}
func (s *service) webhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		errReply(w, 405, "POST required")
		return
	}
	if s.cfg.WebhookKey == "" {
		errReply(w, 503, "webhook not configured")
		return
	}
	payload, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 256<<10))
	if e != nil {
		errReply(w, 413, "request too large")
		return
	}
	if !verifySignature(s.cfg.WebhookKey, r.Header.Get("Stripe-Signature"), payload, time.Now()) {
		errReply(w, 400, "invalid webhook signature")
		return
	}
	var event struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object map[string]any `json:"object"`
		} `json:"data"`
	}
	if e = json.Unmarshal(payload, &event); e != nil || !strings.HasPrefix(event.ID, "evt_") {
		errReply(w, 400, "invalid event")
		return
	}
	s.mu.Lock()
	_, done := s.db.Events[event.ID]
	s.mu.Unlock()
	if done {
		jsonReply(w, 200, map[string]bool{"ok": true})
		return
	}
	sid := ""
	switch event.Type {
	case "checkout.session.completed":
		sid = field(event.Data.Object, "subscription")
		if sid == "" { // Future asynchronous payment methods await subscription/invoice events.
			s.recordEvent(event.ID)
			jsonReply(w, 200, map[string]bool{"ok": true})
			return
		}
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		sid = field(event.Data.Object, "id")
	case "invoice.paid", "invoice.payment_failed":
		sid = field(event.Data.Object, "subscription")
		if sid == "" {
			sid = field(obj(obj(event.Data.Object, "parent"), "subscription_details"), "subscription")
		}
	default:
		s.recordEvent(event.ID)
		jsonReply(w, 200, map[string]bool{"ok": true})
		return
	}
	if !strings.HasPrefix(sid, "sub_") {
		s.recordEvent(event.ID)
		jsonReply(w, 200, map[string]bool{"ok": true})
		return
	}
	if e = s.reconcileSubscription(r, sid); e != nil {
		log.Printf("stripe reconcile failed: %v", e)
		errReply(w, 503, "retry webhook")
		return
	}
	s.recordEvent(event.ID)
	jsonReply(w, 200, map[string]bool{"ok": true})
}
func (s *service) recordEvent(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Events[id] = time.Now().Unix()
	if len(s.db.Events) > 4000 {
		cut := time.Now().Add(-45 * 24 * time.Hour).Unix()
		for k, v := range s.db.Events {
			if v < cut {
				delete(s.db.Events, k)
			}
		}
	}
	if e := s.persist(); e != nil {
		log.Printf("unable to save webhook idempotency state: %v", e)
	}
}
func (s *service) reconcileSubscription(r *http.Request, subID string) error {
	sub, e := s.stripeGet(r, "/v1/subscriptions/"+url.PathEscape(subID))
	if e != nil {
		return e
	}
	if field(sub, "id") != subID {
		return errors.New("subscription ID mismatch")
	}
	pid := field(obj(sub, "metadata"), "purchase_id")
	if pid == "" {
		return nil
	}
	foundPrice := false
	items, _ := obj(sub, "items")["data"].([]any)
	for _, i := range items {
		m, _ := i.(map[string]any)
		pr := obj(m, "price")
		if field(pr, "id") == s.cfg.PriceID {
			foundPrice = true
		}
	}
	if !foundPrice {
		return errors.New("subscription has unexpected price")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.db.Purchases[pid]
	if p == nil {
		return nil
	} // do not grant unrecognized transactions
	existing := p.SubscriptionID
	if existing != "" && existing != subID {
		return errors.New("subscription swap rejected")
	}
	p.SubscriptionID = subID
	p.CustomerID = field(sub, "customer")
	status := field(sub, "status")
	p.Active = status == "active" // no trial period; only actual paid subscriptions qualify.
	p.Paid = p.Paid || p.Active
	// Stripe subscription current_period_end may be absent with newer API versions. Use
	// period_end on the subscription item as a fallback; fail closed if neither exists.
	exp := int64(0)
	if f, ok := sub["current_period_end"].(float64); ok {
		exp = int64(f)
	}
	for _, i := range items {
		m, _ := i.(map[string]any)
		if f, ok := m["current_period_end"].(float64); ok && int64(f) > exp {
			exp = int64(f)
		}
	}
	if exp == 0 {
		p.Active = false
	} else {
		p.Expires = exp
	}
	if p.Active && p.LicenseHash == "" {
		k := "SAWCHA-PLUS-" + strings.ToUpper(token(25))
		enc, e := s.encrypt(k)
		if e != nil {
			return e
		}
		p.LicenseHash = mac(s.cfg.DataKey, "license", k)
		p.LicenseEnc = enc
		p.LicenseLast4 = k[len(k)-4:]
	}
	return s.persist()
}
func main() {
	key, e := hex.DecodeString(strings.TrimSpace(os.Getenv("SAWCHA_DATA_KEY")))
	if e != nil {
		log.Fatal("invalid SAWCHA_DATA_KEY encoding")
	}
	giftPub, giftErr := base64.RawStdEncoding.DecodeString(strings.TrimSpace(os.Getenv("SAWCHA_GIFT_PUBLIC_KEY")))
	if os.Getenv("SAWCHA_GIFT_PUBLIC_KEY") != "" && (giftErr != nil || len(giftPub) != ed25519.PublicKeySize) {
		log.Fatal("invalid complimentary gift verification public key")
	}
	cfg := config{GiftPublicKey: giftPub, DataKey: key, DataDir: os.Getenv("SAWCHA_DATA_DIR"), BaseURL: strings.TrimRight(os.Getenv("SAWCHA_PUBLIC_BASE_URL"), "/"), StripeKey: os.Getenv("STRIPE_SECRET_KEY"), PriceID: os.Getenv("STRIPE_PLUS_PRICE_ID"), WebhookKey: os.Getenv("STRIPE_WEBHOOK_SECRET")}
	s, e := newService(cfg)
	if e != nil {
		log.Fatal(e)
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{Addr: ":" + port, Handler: s.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 45 * time.Second}
	log.Printf("SAWCHA Plus started on port %s; checkout configured=%v", port, s.checkoutReady())
	log.Fatal(server.ListenAndServe())
}
