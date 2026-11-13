//go:build windows

package main

// The per-installation identity is protected by Windows DPAPI. Copying the
// ordinary settings file to another PC no longer copies an accepted identity.
import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"unsafe"
)

type deviceBlob struct {
	Size uint32
	Data *byte
}

var deviceCrypt32 = syscall.NewLazyDLL("crypt32.dll")
var deviceProtect = deviceCrypt32.NewProc("CryptProtectData")
var deviceUnprotect = deviceCrypt32.NewProc("CryptUnprotectData")
var deviceFree = syscall.NewLazyDLL("kernel32.dll").NewProc("LocalFree")
var devicePattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func deviceDPAPI(raw []byte, protect bool) ([]byte, error) {
	if len(raw) == 0 {
		return nil, errors.New("empty device identity")
	}
	input := deviceBlob{Size: uint32(len(raw)), Data: &raw[0]}
	var out deviceBlob
	var ok uintptr
	if protect {
		ok, _, _ = deviceProtect.Call(uintptr(unsafe.Pointer(&input)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&out)))
	} else {
		ok, _, _ = deviceUnprotect.Call(uintptr(unsafe.Pointer(&input)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&out)))
	}
	if ok == 0 || out.Data == nil || out.Size == 0 {
		return nil, errors.New("cannot unlock Windows-protected installation identity")
	}
	defer deviceFree.Call(uintptr(unsafe.Pointer(out.Data)))
	b := unsafe.Slice(out.Data, int(out.Size))
	result := make([]byte, len(b))
	copy(result, b)
	return result, nil
}
func loadInstallationID(legacy string) (string, error) {
	file := filepath.Join(appFolder(), "install.id.dpapi")
	raw, e := os.ReadFile(file)
	if e == nil {
		v, err := deviceDPAPI(raw, false)
		if err != nil {
			return "", err
		}
		id := string(v)
		if !devicePattern.MatchString(id) {
			return "", errors.New("invalid installation ID")
		}
		return id, nil
	}
	if !os.IsNotExist(e) {
		return "", e
	}
	id := legacy
	if !devicePattern.MatchString(id) {
		data := make([]byte, 32)
		if _, e = rand.Read(data); e != nil {
			return "", e
		}
		id = hex.EncodeToString(data)
	}
	sealed, e := deviceDPAPI([]byte(id), true)
	if e != nil {
		return "", e
	}
	if e = os.MkdirAll(filepath.Dir(file), 0700); e != nil {
		return "", e
	}
	f, e := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return "", e
	}
	if _, e = f.Write(sealed); e != nil {
		f.Close()
		return "", e
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	return id, nil
}
