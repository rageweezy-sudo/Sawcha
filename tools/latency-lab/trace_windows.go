//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func captureKernelTrace(ctx context.Context, duration time.Duration) (string, error) {
	// WPR is a Microsoft-supplied ETW controller. This does not install a driver,
	// disable kernel security, alter priorities or read arbitrary kernel memory.
	root := os.Getenv("LOCALAPPDATA")
	if root == "" {
		return "", errors.New("LOCALAPPDATA is unavailable")
	}
	dir := filepath.Join(root, "Sawcha", "LatencyLab", "traces")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "trace-"+time.Now().Format("20060102-150405")+".etl")
	run := func(timeout time.Duration, args ...string) error {
		child, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		command := exec.CommandContext(child, "wpr.exe", args...)
		out, err := command.CombinedOutput()
		if err != nil {
			return fmt.Errorf("WPR %v failed (%v): %s. Run the app as administrator if ETW requires elevation", args, err, string(out))
		}
		return nil
	}
	if err := run(12*time.Second, "-start", "GeneralProfile", "-filemode"); err != nil {
		return "", err
	}
	started := true
	defer func() {
		if started {
			cleanup, stop := context.WithTimeout(context.Background(), 12*time.Second)
			defer stop()
			_ = exec.CommandContext(cleanup, "wpr.exe", "-cancel").Run()
		}
	}()
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if err := run(25*time.Second, "-stop", path); err != nil {
		return "", err
	}
	started = false
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		return "", errors.New("ETW trace file was not created; verify WPR installation and Windows permissions")
	}
	return path, nil
}
