//go:build !windows

package main

import (
	"errors"
	"os/exec"
)

func prepareHidden(_ *exec.Cmd)     {}
func systemMouseSpeed() int         { return 10 }
func setSystemMouseSpeed(int) error { return errors.New("Windows only") }
func adminCheck() bool              { return false }
func launchWindow(string)           {}
