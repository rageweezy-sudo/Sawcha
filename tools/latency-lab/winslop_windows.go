//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Every command in the fixed, embedded script is strictly read-only:
// Get-ItemPropertyValue, Get-AppxPackage and property enumeration.
// No network access, no elevation, no scripts downloaded or executed.
const winSlopReadOnlyScript = `
$ErrorActionPreference='SilentlyContinue'
$ProgressPreference='SilentlyContinue'
$checks = @(
 @{Id='gameDvr';Path='Registry::HKEY_CURRENT_USER\System\GameConfigStore';Value='GameDVR_Enabled'},
 @{Id='appCapture';Path='Registry::HKEY_CURRENT_USER\SOFTWARE\Microsoft\Windows\CurrentVersion\GameDVR';Value='AppCaptureEnabled'},
 @{Id='widgets';Path='Registry::HKEY_CURRENT_USER\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\Advanced';Value='TaskbarDa'},
 @{Id='tailored';Path='Registry::HKEY_CURRENT_USER\SOFTWARE\Microsoft\Windows\CurrentVersion\Privacy';Value='TailoredExperiencesWithDiagnosticDataEnabled'},
 @{Id='consumer';Path='Registry::HKEY_LOCAL_MACHINE\SOFTWARE\Policies\Microsoft\Windows\CloudContent';Value='DisableWindowsConsumerFeatures'},
 @{Id='softLanding';Path='Registry::HKEY_CURRENT_USER\SOFTWARE\Microsoft\Windows\CurrentVersion\ContentDeliveryManager';Value='SoftLandingEnabled'},
 @{Id='startSuggestions';Path='Registry::HKEY_CURRENT_USER\SOFTWARE\Microsoft\Windows\CurrentVersion\ContentDeliveryManager';Value='SubscribedContent-338388Enabled'},
 @{Id='lockScreenTips';Path='Registry::HKEY_CURRENT_USER\SOFTWARE\Microsoft\Windows\CurrentVersion\ContentDeliveryManager';Value='RotatingLockScreenEnabled'},
 @{Id='searchSuggestions';Path='Registry::HKEY_CURRENT_USER\SOFTWARE\Policies\Microsoft\Windows\Explorer';Value='DisableSearchBoxSuggestions'},
 @{Id='copilotPolicy';Path='Registry::HKEY_CURRENT_USER\SOFTWARE\Policies\Microsoft\Windows\WindowsCopilot';Value='TurnOffWindowsCopilot'},
 @{Id='recallPolicy';Path='Registry::HKEY_LOCAL_MACHINE\SOFTWARE\Policies\Microsoft\Windows\WindowsAI';Value='DisableAIDataAnalysis'},
 @{Id='advertisingId';Path='Registry::HKEY_CURRENT_USER\SOFTWARE\Microsoft\Windows\CurrentVersion\AdvertisingInfo';Value='Enabled'}
)
$registry=@{}
foreach($c in $checks) {
 try {
   $v=Get-ItemPropertyValue -LiteralPath $c.Path -Name $c.Value -ErrorAction Stop
   if($null -ne $v -and ($v -is [int] -or $v -is [long] -or $v -is [string])) {
     $registry[$c.Id] = [string]$v
   }
 } catch { }
}
$wanted = @('Microsoft.Copilot','Microsoft.MicrosoftSolitaireCollection','Clipchamp.Clipchamp','Microsoft.WindowsFeedbackHub','Microsoft.BingNews','Microsoft.GetHelp','Microsoft.YourPhone','Microsoft.XboxGamingOverlay','Microsoft.BingWeather','Microsoft.WindowsMaps','Microsoft.People','Microsoft.ZuneMusic','Microsoft.ZuneVideo','Microsoft.Getstarted','Microsoft.549981C3F5F10','Microsoft.OutlookForWindows')
$packages=@()
foreach($name in $wanted) {
 try {
   $installed=@(Get-AppxPackage -Name $name -ErrorAction Stop)
   if($installed.Count -gt 0){$packages+= $name}
 } catch { }
}
$startup = $null
try {
 $row=Get-ItemProperty -LiteralPath 'Registry::HKEY_CURRENT_USER\SOFTWARE\Microsoft\Windows\CurrentVersion\Run' -ErrorAction Stop
 $startup=@($row.PSObject.Properties | Where-Object { $_.Name -notlike 'PS*' }).Count
} catch { }
[ordered]@{registry=$registry;installedOptionalApps=@($packages);userStartupEntryCount=$startup} | ConvertTo-Json -Depth 4 -Compress
`

func readWinSlopSnapshot(ctx context.Context) (map[string]any, error) {
	// Fixed source only, not arbitrary user-supplied commands or downloaded code.
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", winSlopReadOnlyScript)
	raw, err := cmd.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, errors.New("inspection timed out; try again outside a game")
		}
		return nil, fmt.Errorf("Windows policy inspection unavailable: %w", err)
	}
	b := strings.TrimSpace(strings.TrimPrefix(string(raw), "\ufeff"))
	if len(b) > 512*1024 {
		return nil, errors.New("Windows policy response unexpectedly large")
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(b), &out); err != nil {
		return nil, fmt.Errorf("invalid Windows policy JSON: %w", err)
	}
	return out, nil
}
