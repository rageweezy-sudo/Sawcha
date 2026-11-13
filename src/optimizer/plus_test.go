package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPlusURLValidation(t *testing.T) {
	for _, raw := range []string{"http://example.com", "https://someone:secret@example.com", "https://example.com/path", "https://example.com?foo=1", "file:///etc/passwd"} {
		if _, e := plusURL(raw); e == nil {
			t.Errorf("URL should be rejected: %s", raw)
		}
	}
	for _, raw := range []string{"https://plus.sawcha.example", "http://localhost:8432", "http://127.0.0.1:8432"} {
		if _, e := plusURL(raw); e != nil {
			t.Errorf("valid URL rejected: %s, %s", raw, e)
		}
	}
}
func TestPlusClientVerificationAndStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Error("should use POST")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/verify":
			_ = json.NewEncoder(w).Encode(plusResponse{Active: true, Plan: "plus", Expires: "2027-01-01T00:00:00Z"})
		case "/v1/checkout/status":
			_ = json.NewEncoder(w).Encode(plusResponse{Active: true, Plan: "plus", Expires: "2027-01-01T00:00:00Z", Key: "SAWCHA-PLUS-" + strings.Repeat("A", 50)})
		default:
			http.Error(w, "not found", 404)
		}
	}))
	defer srv.Close()
	// Tests run against loopback only. The release app enforces HTTPS remotely.
	p := plusSettings{Server: srv.URL, DeviceID: strings.Repeat("a", 64), LicenseKey: "SAWCHA-PLUS-" + strings.Repeat("A", 50), ClaimToken: strings.Repeat("b", 64)}
	active, e := plusVerify(context.Background(), &p)
	if e != nil || !active || p.Plan != "plus" {
		t.Fatalf("verification failed %v %+v", e, p)
	}
	// The claim endpoint only returns keys to the browser owner.
	v, e := plusRequest(context.Background(), p, "/v1/checkout/status", map[string]string{"claim_token": p.ClaimToken, "device_id": p.DeviceID})
	if e != nil || !strings.HasPrefix(v.Key, "SAWCHA-PLUS-") {
		t.Fatalf("claim failed: %v %+v", e, v)
	}
}
func TestComplimentaryGiftClientActivatesThroughServer(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	key := "SAWCHA-GIFT-V1." + strings.Repeat("A", 100)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/gift/redeem" && r.URL.Path != "/v1/verify" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(plusResponse{Active: true, Plan: "plus", Source: "complimentary", Message: "Complimentary Pro access verified — no charge", Expires: "2027-01-01T00:00:00Z"})
	}))
	defer server.Close()
	p := plusSettings{Server: server.URL, DeviceID: strings.Repeat("a", 64), LicenseKey: key}
	ok, e := plusActivate(context.Background(), &p)
	if e != nil || !ok || p.Source != "complimentary" {
		t.Fatalf("gift activation failed: %v %+v", e, p)
	}
	if _, e = plusPortal(context.Background(), p); e == nil {
		t.Fatal("complimentary key should not open Stripe portal")
	}
	ok, e = plusVerify(context.Background(), &p)
	if e != nil || !ok {
		t.Fatal("gift verification failed", e)
	}
}
