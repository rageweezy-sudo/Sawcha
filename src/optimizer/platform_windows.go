//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var user32 = syscall.NewLazyDLL("user32.dll")
var spi = user32.NewProc("SystemParametersInfoW")

func prepareHidden(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
func systemMouseSpeed() int {
	var speed uint32
	ok, _, _ := spi.Call(0x70, 0, uintptr(unsafe.Pointer(&speed)), 0)
	if ok == 0 {
		return 0
	}
	return int(speed)
}
func setSystemMouseSpeed(n int) error {
	ok, _, err := spi.Call(0x71, 0, uintptr(n), 3)
	if ok == 0 {
		if err != nil && err != syscall.Errno(0) {
			return err
		}
		return errors.New("SystemParametersInfoW returned false")
	}
	return nil
}
func adminCheck() bool {
	// Membership alone does not imply that the process is elevated; verify token integrity level.
	out, err := exec.Command("whoami.exe", "/groups").Output()
	return err == nil && strings.Contains(string(out), "S-1-16-12288")
}
func launchWindow(address string) {
	paths := []string{}
	for _, p := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA")} {
		if p != "" {
			paths = append(paths, filepath.Join(p, "Microsoft", "Edge", "Application", "msedge.exe"), filepath.Join(p, "Google", "Chrome", "Application", "chrome.exe"))
		}
	}
	for _, p := range paths {
		if fileExists(p) {
			if exec.Command(p, "--app="+address, "--new-window", "--window-size=1380,900").Start() == nil {
				return
			}
		}
	}
	for _, c := range []string{"msedge.exe", "chrome.exe"} {
		if p, e := exec.LookPath(c); e == nil {
			if exec.Command(p, "--app="+address, "--new-window", "--window-size=1380,900").Start() == nil {
				return
			}
		}
	}
	_ = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", address).Start()
}
