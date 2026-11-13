//go:build windows

package main

import (
	"strings"
	"testing"
)

func TestWindowsRemovalCommandProtectsSystemAndUsesExactIdentity(t *testing.T) {
	for _, app := range removableApps {
		if !strings.Contains(removeOptionalAppScript, "'"+app.ID+"'") {
			t.Fatalf("Windows script missing allowlist entry %q", app.ID)
		}
		if !strings.Contains(winSlopReadOnlyScript, "'"+app.ID+"'") {
			t.Fatalf("Windows inventory missing optional package %q", app.ID)
		}
	}
	for _, needed := range []string{"Remove-AppxPackage -Package $full", "-ceq $name", "NonRemovable", "IsFramework", "IsResourcePackage", "Get-AppxPackage -Name $name", "$env:SAWCHA_REMOVAL_SELECTION"} {
		if !strings.Contains(removeOptionalAppScript, needed) {
			t.Fatalf("unsafe removal script is missing %q", needed)
		}
	}
	for _, blocked := range []string{"-AllUsers", "Remove-AppxProvisionedPackage", "Remove-Item", "reg.exe", "Disable-WindowsOptionalFeature", "Remove-Service", "Set-MpPreference", "Invoke-WebRequest"} {
		if strings.Contains(removeOptionalAppScript, blocked) {
			t.Fatalf("disallowed command in removal script: %s", blocked)
		}
	}
}
