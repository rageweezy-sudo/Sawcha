//go:build !windows

package main

import (
	"context"
	"errors"
)

func readWinSlopSnapshot(_ context.Context) (map[string]any, error) {
	return nil, errors.New("Windows 10 or 11 required")
}
