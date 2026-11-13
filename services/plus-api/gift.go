package main

// Complimentary Pro access. Only a private, offline Ed25519 signing key can
// mint or revoke grants; the public service holds only the verification key.
// Never embed the private key in a customer binary or public repository.

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const giftPrefix = "SAWCHA-GIFT-V1."
const revokePrefix = "SAWCHA-REVOKE-V1."
const giftContext = "sawcha-gift-v1:"
const revokeContext = "sawcha-gift-revoke-v1:"

var grantIDValid = regexp.MustCompile(`^[a-f0-9]{32}$`)

type giftGrant struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	IssuedAt    int64  `json:"issued_at"`
	ExpiresAt   int64  `json:"expires_at"`
	DeviceLimit int    `json:"device_limit"`
}
type giftRevoke struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	IssuedAt  int64  `json:"issued_at"`
	ExpiresAt int64  `json:"expires_at"`
}
type giftUse struct {
	DeviceHash string `json:"device_hash"`
	FirstUsed  int64  `json:"first_used"`
}

func parseSigned[T any](public ed25519.PublicKey, token, prefix, context string) (T, error) {
	var value T
	if len(public) != ed25519.PublicKeySize || len(token) > 1600 || !strings.HasPrefix(token, prefix) {
		return value, errors.New("signature verification unavailable")
	}
	parts := strings.Split(strings.TrimPrefix(token, prefix), ".")
	if len(parts) != 2 {
		return value, errors.New("invalid token layout")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(payload) > 600 {
		return value, errors.New("invalid token payload")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(sig) != ed25519.SignatureSize || !ed25519.Verify(public, append([]byte(context), payload...), sig) {
		return value, errors.New("invalid signature")
	}
	if err = json.Unmarshal(payload, &value); err != nil {
		return value, errors.New("invalid token data")
	}
	return value, nil
}
func grantFromToken(public ed25519.PublicKey, token string, now time.Time) (giftGrant, error) {
	g, e := parseSigned[giftGrant](public, token, giftPrefix, giftContext)
	if e != nil {
		return g, e
	}
	if !grantIDValid.MatchString(g.ID) || g.Kind != "complimentary-pro" || g.DeviceLimit != 1 || g.IssuedAt > now.Unix()+300 || g.IssuedAt < now.Add(-366*24*time.Hour).Unix() || g.ExpiresAt <= now.Unix() || g.ExpiresAt <= g.IssuedAt || g.ExpiresAt-g.IssuedAt > 366*24*3600 {
		return g, errors.New("invalid or expired complimentary grant")
	}
	return g, nil
}
func (s *service) giftStatus(key, device string, redeem bool) map[string]any {
	out := map[string]any{"active": false, "plan": "basic", "features": []string{}, "message": "Invalid, expired or unclaimed complimentary key"}
	if !validDevice.MatchString(device) {
		return out
	}
	grant, e := grantFromToken(s.cfg.GiftPublicKey, key, time.Now())
	if e != nil {
		return out
	}
	if s.db.RevokedGifts[grant.ID] != 0 {
		out["message"] = "Complimentary key has been revoked"
		return out
	}
	dh := mac(s.cfg.DataKey, "device", device)
	old := s.db.Gifts[grant.ID]
	if old == nil && !redeem {
		out["message"] = "Redeem the complimentary key to activate it"
		return out
	}
	if old != nil && !same(old.DeviceHash, dh) {
		out["message"] = "Complimentary key is already assigned to another device"
		return out
	}
	if old == nil {
		s.db.Gifts[grant.ID] = &giftUse{DeviceHash: dh, FirstUsed: time.Now().Unix()}
		if e = s.persist(); e != nil {
			delete(s.db.Gifts, grant.ID)
			out["message"] = "Unable to save activation"
			return out
		}
	}
	out["active"] = true
	out["plan"] = "plus"
	out["source"] = "complimentary"
	out["features"] = []string{"game_profiles_advanced", "frame_analysis", "plus_guidance"}
	out["expires_at"] = time.Unix(grant.ExpiresAt, 0).UTC().Format(time.RFC3339)
	out["message"] = "Complimentary Pro access verified — no charge"
	return out
}
func (s *service) redeemGift(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key      string `json:"key"`
		DeviceID string `json:"device_id"`
	}
	if !body(w, r, &in) {
		return
	}
	s.mu.Lock()
	out := s.giftStatus(strings.TrimSpace(in.Key), in.DeviceID, true)
	s.mu.Unlock()
	jsonReply(w, 200, out)
}

// Revocation can only be authorized by a short-lived, separately signed command.
// No developer password or signing key is ever accepted or stored by the service.
func (s *service) revokeGift(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SignedCommand string `json:"signed_command"`
	}
	if !body(w, r, &in) {
		return
	}
	cmd, e := parseSigned[giftRevoke](s.cfg.GiftPublicKey, strings.TrimSpace(in.SignedCommand), revokePrefix, revokeContext)
	now := time.Now().Unix()
	if e != nil || !grantIDValid.MatchString(cmd.ID) || cmd.Kind != "revoke" || cmd.IssuedAt > now+120 || cmd.IssuedAt < now-300 || cmd.ExpiresAt <= now || cmd.ExpiresAt > now+600 {
		errReply(w, 403, "invalid authorization")
		return
	}
	s.mu.Lock()
	s.db.RevokedGifts[cmd.ID] = now
	e = s.persist()
	s.mu.Unlock()
	if e != nil {
		errReply(w, 500, "database unavailable")
		return
	}
	jsonReply(w, 200, map[string]any{"ok": true, "grant_id": cmd.ID, "revoked": true})
}
func giftID(token string) string {
	// Returns only a fingerprint (not the raw grant) for safe local log output.
	b := sha256.Sum256([]byte(token))
	return hex.EncodeToString(b[:6])
}
