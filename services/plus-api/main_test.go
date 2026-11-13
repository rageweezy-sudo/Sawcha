package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixture(t *testing.T) (*service, *httptest.Server, *string, *string) {
	t.Helper()
	status := "active"
	purchaseID := ""
	var mu sync.Mutex
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Basic "+"c2tfdGVzdF8xMjM6" {
			t.Errorf("expected Stripe Basic auth, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/checkout/sessions":
			_ = r.ParseForm()
			if r.Form.Get("line_items[0][price]") != "price_test" || r.Form.Get("mode") != "subscription" {
				t.Error("checkout not monthly Plus plan")
			}
			_, _ = w.Write([]byte(`{"id":"cs_test_ABC","url":"https://checkout.stripe.com/c/test-example"}`))
		case r.URL.Path == "/v1/subscriptions/sub_ABC":
			mu.Lock()
			st := status
			pid := purchaseID
			mu.Unlock()
			_, _ = fmt.Fprintf(w, `{"id":"sub_ABC","customer":"cus_ABC","status":%q,"metadata":{"purchase_id":%q},"items":{"data":[{"price":{"id":"price_test"},"current_period_end":%d}]}}`, st, pid, time.Now().Add(30*24*time.Hour).Unix())
		case r.URL.Path == "/v1/billing_portal/sessions":
			_, _ = w.Write([]byte(`{"url":"https://billing.stripe.com/p/session-test"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	key := bytes.Repeat([]byte{0x45}, 32)
	svc, e := newService(config{DataKey: key, DataDir: filepath.Join(t.TempDir(), "private"), BaseURL: "https://sawcha.example", PriceID: "price_test", StripeKey: "sk_test_123", WebhookKey: "whsec_test1234567890"})
	if e != nil {
		t.Fatal(e)
	}
	svc.stripeAPI = remote.URL
	return svc, remote, &status, &purchaseID
}
func request(t *testing.T, s *service, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(payload)
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	r.RemoteAddr = "127.0.0.1:13211"
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	return w
}
func sign(secret string, payload []byte) string {
	ts := time.Now().Unix()
	h := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(h, "%d.", ts)
	h.Write(payload)
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(h.Sum(nil)))
}
func sendEvent(t *testing.T, s *service, typ, id string, obj any, valid bool) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"id": id, "type": typ, "data": map[string]any{"object": obj}})
	r := httptest.NewRequest("POST", "/stripe/webhook", bytes.NewReader(b))
	r.RemoteAddr = "127.0.0.1:13211"
	sig := sign(s.cfg.WebhookKey, b)
	if !valid {
		sig = "t=123,v1=00"
	}
	r.Header.Set("Stripe-Signature", sig)
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	return w
}
func TestFullSubscriptionLifecycle(t *testing.T) {
	svc, remote, state, purchaseID := fixture(t)
	defer remote.Close()
	device := strings.Repeat("a", 64)
	w := request(t, svc, "/v1/checkout", map[string]string{"email": "buyer@example.com", "device_id": device})
	if w.Code != 200 {
		t.Fatalf("checkout %d %s", w.Code, w.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	token := out["claim_token"].(string)
	status := request(t, svc, "/v1/checkout/status", map[string]string{"claim_token": token, "device_id": device})
	if !strings.Contains(status.Body.String(), `"active":false`) {
		t.Fatalf("pending did not stay Basic: %s", status.Body.String())
	}
	if got := sendEvent(t, svc, "checkout.session.completed", "evt_invalid", map[string]string{"subscription": "sub_ABC"}, false); got.Code != 400 {
		t.Fatalf("forged webhook accepted: %d", got.Code)
	}
	svc.mu.Lock()
	for id, p := range svc.db.Purchases {
		if p.SessionID != "cs_test_ABC" {
			t.Fatal("checkout session not saved")
		}
		*purchaseID = id
	}
	svc.mu.Unlock()
	good := sendEvent(t, svc, "customer.subscription.updated", "evt_ok_1", map[string]string{"id": "sub_ABC"}, true)
	if good.Code != 200 {
		t.Fatalf("verified webhook failed: %d %s", good.Code, good.Body.String())
	}
	status = request(t, svc, "/v1/checkout/status", map[string]string{"claim_token": token, "device_id": device})
	if status.Code != 200 {
		t.Fatal(status.Body.String())
	}
	_ = json.Unmarshal(status.Body.Bytes(), &out)
	key := out["license_key"].(string)
	if !strings.HasPrefix(key, "SAWCHA-PLUS-") {
		t.Fatal("invalid Plus key")
	}
	verified := request(t, svc, "/v1/verify", map[string]string{"key": key, "device_id": device})
	if !strings.Contains(verified.Body.String(), `"active":true`) {
		t.Fatal(verified.Body.String())
	}
	blocked := request(t, svc, "/v1/verify", map[string]string{"key": key, "device_id": strings.Repeat("b", 64)})
	if !strings.Contains(blocked.Body.String(), `"active":false`) {
		t.Fatal("different device accepted")
	}
	portal := request(t, svc, "/v1/portal", map[string]string{"key": key, "device_id": device})
	if portal.Code != 200 {
		t.Fatalf("portal failed: %d %s", portal.Code, portal.Body.String())
	}
	*state = "canceled"
	canceled := sendEvent(t, svc, "customer.subscription.deleted", "evt_ok_2", map[string]string{"id": "sub_ABC"}, true)
	if canceled.Code != 200 {
		t.Fatal(canceled.Body.String())
	}
	verified = request(t, svc, "/v1/verify", map[string]string{"key": key, "device_id": device})
	if !strings.Contains(verified.Body.String(), `"active":false`) {
		t.Fatal("cancellation did not revoke")
	}
	again := sendEvent(t, svc, "customer.subscription.deleted", "evt_ok_2", map[string]string{"id": "sub_ABC"}, true)
	if again.Code != 200 {
		t.Fatal("duplicate should be harmless")
	}
}
func TestStorageAndSignature(t *testing.T) {
	svc, remote, _, _ := fixture(t)
	defer remote.Close()
	if verifySignature(svc.cfg.WebhookKey, "t=123,v1=bad", []byte("{}"), time.Now()) {
		t.Fatal("old/forged signature accepted")
	}
	payload := []byte(`{"type":"ping"}`)
	if !verifySignature(svc.cfg.WebhookKey, sign(svc.cfg.WebhookKey, payload), payload, time.Now()) {
		t.Fatal("valid signature rejected")
	}
	key := "SAWCHA-PLUS-" + token(25)
	encrypted, e := svc.encrypt(key)
	if e != nil {
		t.Fatal(e)
	}
	plain, e := svc.decrypt(encrypted)
	if e != nil || plain != key {
		t.Fatal("AES-GCM roundtrip failed")
	}
	if strings.Contains(encrypted, key) {
		t.Fatal("key stored plaintext")
	}
	if _, e = url.Parse(svc.cfg.BaseURL); e != nil {
		t.Fatal(e)
	}
}
