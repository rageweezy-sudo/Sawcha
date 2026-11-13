//go:build !windows

package main

import (
	"context"
	"errors"
)

func windowsSnapshot(ctx context.Context, deep bool) (map[string]any, error) {
	return nil, errors.New("Windows 10/11 required")
}
