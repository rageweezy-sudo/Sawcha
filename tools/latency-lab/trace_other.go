//go:build !windows

package main

import (
	"context"
	"errors"
	"time"
)

func captureKernelTrace(ctx context.Context, duration time.Duration) (string, error) {
	return "", errors.New("Windows WPR is required")
}
