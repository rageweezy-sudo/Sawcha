//go:build !windows

package main

import (
	"context"
	"errors"
)

func removeWindowsOptionalApps(_ context.Context, _ []optionalApp) (appRemovalReceipt, error) {
	return appRemovalReceipt{}, errors.New("Windows 10 or 11 required")
}
