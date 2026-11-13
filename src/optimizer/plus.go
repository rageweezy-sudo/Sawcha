package main

// Public, customer-only Plus API client. It never contains a Stripe secret, webhook
// secret, license-signing key, billing key, or developer credential.
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The hosted API URL can be provided at runtime and saved in the app's Plus
// screen. Only HTTPS URLs are allowed for remote servers.
const defaultPlusURL = ""

type plusSettings struct {
	Server     string `json:"server"`
	Email      string `json:"email,omitempty"`
	DeviceID   string `json:"-"`
	ClaimToken string `json:"claim_token,omitempty"`
	LicenseKey string `json:"license_key,omitempty"`
	Plan       string `json:"-"`
	Source     string `json:"source,omitempty"`
	Expires    string `json:"-"`
	Message    string `json:"-"`
}
type plusResponse struct {
	Active      bool   `json:"active"`
	Source      string `json:"source"`
	Plan        string `json:"plan"`
	Expires     string `json:"expires_at"`
	Key         string `json:"license_key"`
	URL         string `json:"url"`
	CheckoutURL string `json:"checkout_url"`
	ClaimToken  string `json:"claim_token"`
	Message     string `json:"message"`
	Error       string `json:"error"`
}

func loadPlus() plusSettings {
	s := plusSettings{Server: defaultPlusURL, Plan: "basic", Message: "Basic (free)"}
	if fromEnv := os.Getenv("SAWCHA_PLUS_URL"); fromEnv != "" {
		s.Server = fromEnv
	}
	b, e := os.ReadFile(filepath.Join(appFolder(), "plus.json"))
	if e == nil {
		_ = json.Unmarshal(b, &s)
	}
	// A legacy release stored a transferable device identifier in JSON.
	// Migrate it once into Windows DPAPI protection, then remove it from JSON.
	var old struct {
		DeviceID string `json:"device_id"`
	}
	_ = json.Unmarshal(b, &old)
	device, err := loadInstallationID(old.DeviceID)
	if err != nil {
		s.Message = "Cannot unlock Windows installation identity: " + err.Error()
	} else {
		s.DeviceID = device
	}
	_ = savePlus(s)
	return s
}
func savePlus(p plusSettings) error {
	f := filepath.Join(appFolder(), "plus.json")
	if e := os.MkdirAll(filepath.Dir(f), 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(p, "", "  ")
	if e != nil {
		return e
	}
	temp := f + ".tmp"
	if e = os.WriteFile(temp, b, 0600); e != nil {
		return e
	}
	return os.Rename(temp, f)
}
func plusURL(base string) (string, error) {
	parsed, e := url.Parse(strings.TrimSpace(base))
	if e != nil {
		return "", e
	}
	secure := parsed.Scheme == "https" && parsed.Host != ""
	if parsed.Scheme == "http" {
		h := parsed.Hostname()
		ip := net.ParseIP(h)
		secure = h == "localhost" || (ip != nil && ip.IsLoopback())
	}
	if !secure || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" || parsed.Port() == "0" {
		return "", errors.New("use a valid HTTPS Plus server URL (HTTP allowed on localhost only)")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}
func plusRequest(ctx context.Context, p plusSettings, endpoint string, payload any) (plusResponse, error) {
	base, e := plusURL(p.Server)
	if e != nil {
		return plusResponse{}, e
	}
	b, e := json.Marshal(payload)
	if e != nil {
		return plusResponse{}, e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", base+endpoint, bytes.NewReader(b))
	if e != nil {
		return plusResponse{}, e
	}
	req.Header.Set("Content-Type", "application/json")
	cli := &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := cli.Do(req)
	if e != nil {
		return plusResponse{}, e
	}
	defer resp.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if e != nil {
		return plusResponse{}, e
	}
	var v plusResponse
	if e = json.Unmarshal(raw, &v); e != nil {
		return v, errors.New("unexpected licensing server response")
	}
	if resp.StatusCode != 200 {
		if v.Error == "" {
			v.Error = "license server returned an error"
		}
		return v, fmt.Errorf("%s (HTTP %d)", v.Error, resp.StatusCode)
	}
	return v, nil
}
func plusCheckout(ctx context.Context, p *plusSettings) (string, error) {
	if p.DeviceID == "" {
		return "", errors.New("installation ID unavailable")
	}
	v, e := plusRequest(ctx, *p, "/v1/checkout", map[string]string{"email": strings.TrimSpace(p.Email), "device_id": p.DeviceID})
	if e != nil {
		return "", e
	}
	checkout, e := url.Parse(v.CheckoutURL)
	if e != nil || checkout.Scheme != "https" || checkout.Hostname() != "checkout.stripe.com" || len(v.ClaimToken) != 64 {
		return "", errors.New("invalid checkout response")
	}
	p.ClaimToken = v.ClaimToken
	p.Message = "Checkout created; complete payment in the browser, then check status"
	if e = savePlus(*p); e != nil {
		return "", e
	}
	return v.CheckoutURL, nil
}
func plusPoll(ctx context.Context, p *plusSettings) (bool, error) {
	if p.ClaimToken == "" {
		return false, errors.New("no pending checkout; subscribe first")
	}
	v, e := plusRequest(ctx, *p, "/v1/checkout/status", map[string]string{"claim_token": p.ClaimToken, "device_id": p.DeviceID})
	if e != nil {
		return false, e
	}
	if v.Active && v.Plan == "plus" && strings.HasPrefix(v.Key, "SAWCHA-PLUS-") {
		p.LicenseKey = v.Key
		p.Plan = "plus"
		p.Source = "subscription"
		p.Expires = v.Expires
		p.Message = "Plus active"
		if e = savePlus(*p); e != nil {
			return false, e
		}
		return true, nil
	}
	p.Plan = "basic"
	p.Message = v.Message
	return false, nil
}
func plusVerify(ctx context.Context, p *plusSettings) (bool, error) {
	if p.LicenseKey == "" {
		p.Plan = "basic"
		p.Message = "No Plus key saved"
		return false, nil
	}
	v, e := plusRequest(ctx, *p, "/v1/verify", map[string]string{"key": strings.TrimSpace(p.LicenseKey), "device_id": p.DeviceID})
	if e != nil {
		p.Plan = "basic"
		p.Message = "Could not verify Plus. Basic features remain available."
		return false, e
	}
	if v.Active && v.Plan == "plus" {
		p.Plan = "plus"
		p.Source = v.Source
		p.Expires = v.Expires
		p.Message = v.Message
		_ = savePlus(*p)
		return true, nil
	}
	p.Plan = "basic"
	p.Source = ""
	p.Expires = ""
	p.Message = v.Message
	return false, nil
}

// Activates a developer-signed complimentary grant with the same server that
// validates paid memberships. Grants are never locally self-asserted.
func plusActivate(ctx context.Context, p *plusSettings) (bool, error) {
	if !strings.HasPrefix(strings.TrimSpace(p.LicenseKey), "SAWCHA-GIFT-V1.") {
		return plusVerify(ctx, p)
	}
	v, e := plusRequest(ctx, *p, "/v1/gift/redeem", map[string]string{"key": strings.TrimSpace(p.LicenseKey), "device_id": p.DeviceID})
	if e != nil {
		p.Plan = "basic"
		return false, e
	}
	if !v.Active || v.Plan != "plus" || v.Source != "complimentary" {
		p.Plan = "basic"
		return false, errors.New(v.Message)
	}
	p.Plan = "plus"
	p.Source = "complimentary"
	p.Expires = v.Expires
	p.Message = v.Message
	if e = savePlus(*p); e != nil {
		return false, e
	}
	return true, nil
}
func plusPortal(ctx context.Context, p plusSettings) (string, error) {
	if p.Source == "complimentary" {
		return "", errors.New("Complimentary Pro has no Stripe subscription. Ask the developer for renewal.")
	}

	v, e := plusRequest(ctx, p, "/v1/portal", map[string]string{"key": p.LicenseKey, "device_id": p.DeviceID})
	if e != nil {
		return "", e
	}
	u, e := url.Parse(v.URL)
	if e != nil || u.Scheme != "https" || u.Hostname() != "billing.stripe.com" {
		return "", errors.New("unexpected Stripe billing portal URL")
	}
	return v.URL, nil
}
