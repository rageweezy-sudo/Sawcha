package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func signedGift(priv ed25519.PrivateKey, g giftGrant) string {
	b, _ := json.Marshal(g)
	sig := ed25519.Sign(priv, append([]byte(giftContext), b...))
	return giftPrefix + base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(sig)
}
func signedRevocation(priv ed25519.PrivateKey, id string) string {
	now := time.Now()
	b, _ := json.Marshal(giftRevoke{ID: id, Kind: "revoke", IssuedAt: now.Unix(), ExpiresAt: now.Add(2 * time.Minute).Unix()})
	sig := ed25519.Sign(priv, append([]byte(revokeContext), b...))
	return revokePrefix + base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(sig)
}
func TestGiftLifecycleAndNoPaymentRequired(t *testing.T) {
	svc, server, _, _ := fixture(t)
	defer server.Close()
	pub, priv, _ := ed25519.GenerateKey(nil)
	svc.cfg.GiftPublicKey = pub
	id := "1234567890abcdef1234567890abcdef"
	now := time.Now()
	key := signedGift(priv, giftGrant{ID: id, Kind: "complimentary-pro", IssuedAt: now.Add(-time.Minute).Unix(), ExpiresAt: now.Add(24 * time.Hour).Unix(), DeviceLimit: 1})
	dev := strings.Repeat("a", 64)
	// A gift must not silently activate via the read-only endpoint.
	if r := request(t, svc, "/v1/verify", map[string]string{"key": key, "device_id": dev}); strings.Contains(r.Body.String(), `"active":true`) {
		t.Fatal("unclaimed gift activated without redeem")
	}
	if r := request(t, svc, "/v1/gift/redeem", map[string]string{"key": key, "device_id": dev}); r.Code != 200 || !strings.Contains(r.Body.String(), `"active":true`) {
		t.Fatalf("gift redeem: %d %s", r.Code, r.Body.String())
	}
	if r := request(t, svc, "/v1/verify", map[string]string{"key": key, "device_id": dev}); !strings.Contains(r.Body.String(), `"active":true`) {
		t.Fatalf("verification failed: %s", r.Body.String())
	}
	if r := request(t, svc, "/v1/gift/redeem", map[string]string{"key": key, "device_id": strings.Repeat("b", 64)}); strings.Contains(r.Body.String(), `"active":true`) {
		t.Fatal("gift copied to second device")
	}
	altered := key[:len(key)-2] + "xx"
	if r := request(t, svc, "/v1/gift/redeem", map[string]string{"key": altered, "device_id": strings.Repeat("b", 64)}); strings.Contains(r.Body.String(), `"active":true`) {
		t.Fatal("tampered signature accepted")
	}
	if r := request(t, svc, "/v1/gift/revoke", map[string]string{"signed_command": "untrusted"}); r.Code != 403 {
		t.Fatalf("unsigned revoke accepted: %d", r.Code)
	}
	if r := request(t, svc, "/v1/gift/revoke", map[string]string{"signed_command": signedRevocation(priv, id)}); r.Code != 200 {
		t.Fatalf("signed revoke rejected: %d %s", r.Code, r.Body.String())
	}
	if r := request(t, svc, "/v1/verify", map[string]string{"key": key, "device_id": dev}); strings.Contains(r.Body.String(), `"active":true`) {
		t.Fatal("revocation not enforced")
	}
	reopened, err := newService(svc.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.db.RevokedGifts[id] == 0 {
		t.Fatal("revocation not persisted")
	}
}
func TestGiftExpiredForgedAndDisabled(t *testing.T) {
	svc, remote, _, _ := fixture(t)
	defer remote.Close()
	pub, priv, _ := ed25519.GenerateKey(nil)
	id := "abcdef0123456789abcdef0123456789"
	now := time.Now()
	exp := signedGift(priv, giftGrant{ID: id, Kind: "complimentary-pro", IssuedAt: now.Add(-48 * time.Hour).Unix(), ExpiresAt: now.Add(-time.Hour).Unix(), DeviceLimit: 1})
	dev := strings.Repeat("d", 64)
	svc.cfg.GiftPublicKey = pub
	if r := request(t, svc, "/v1/gift/redeem", map[string]string{"key": exp, "device_id": dev}); strings.Contains(r.Body.String(), `"active":true`) {
		t.Fatal("expired gift accepted")
	}
	svc.cfg.GiftPublicKey = nil
	good := signedGift(priv, giftGrant{ID: id, Kind: "complimentary-pro", IssuedAt: now.Add(-time.Minute).Unix(), ExpiresAt: now.Add(time.Hour).Unix(), DeviceLimit: 1})
	if r := request(t, svc, "/v1/gift/redeem", map[string]string{"key": good, "device_id": dev}); strings.Contains(r.Body.String(), `"active":true`) {
		t.Fatal("gift accepted without publisher key")
	}
	if r := request(t, svc, "/v1/gift/revoke", map[string]string{"signed_command": "no"}); r.Code != 403 {
		t.Fatal("unsigned revoke succeeded")
	}
	// Public endpoint is not a developer key minting API.
	r := httptest.NewRequest(http.MethodPost, "/v1/gift/issue", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	svc.routes().ServeHTTP(w, r)
	if w.Code != 404 && w.Code != 405 {
		t.Fatalf("public issuer endpoint unexpectedly exists: %d", w.Code)
	}
}
