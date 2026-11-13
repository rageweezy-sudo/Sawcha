//go:build !windows

package main

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
)

// Used for cross-platform unit tests only. Production is Windows DPAPI.
func loadInstallationID(legacy string) (string, error) {
	if len(legacy) == 64 {
		return legacy, nil
	}
	f := filepath.Join(appFolder(), "install.id.test")
	if b, e := os.ReadFile(f); e == nil && len(b) == 64 {
		return string(b), nil
	}
	raw := make([]byte, 32)
	if _, e := rand.Read(raw); e != nil {
		return "", e
	}
	id := hex.EncodeToString(raw)
	if e := os.MkdirAll(filepath.Dir(f), 0700); e != nil {
		return "", e
	}
	if e := os.WriteFile(f, []byte(id), 0600); e != nil {
		return "", e
	}
	return id, nil
}
