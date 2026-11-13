//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Current-user AppX removal only. IDs are passed in a dedicated environment
// variable after Go allowlist validation (no shell interpolation). PowerShell
// independently checks its OWN allowlist and freshly re-queries the package's
// exact identity, nonremovability, framework and resource flags before removal.
const removeOptionalAppScript = `
$ErrorActionPreference='Stop'
$ProgressPreference='SilentlyContinue'
$allowed=@('Microsoft.Copilot','Microsoft.MicrosoftSolitaireCollection','Clipchamp.Clipchamp','Microsoft.WindowsFeedbackHub','Microsoft.BingNews','Microsoft.GetHelp','Microsoft.YourPhone','Microsoft.XboxGamingOverlay','Microsoft.BingWeather','Microsoft.WindowsMaps','Microsoft.People','Microsoft.ZuneMusic','Microsoft.ZuneVideo','Microsoft.Getstarted','Microsoft.549981C3F5F10','Microsoft.OutlookForWindows')
$names=@($env:SAWCHA_REMOVAL_SELECTION -split ',')
$results=@()
foreach($name in $names) {
 $result=[ordered]@{id=$name;status='failed';message='Removal not completed.'}
 if($allowed -cnotcontains $name) {throw 'Unapproved application; refusing operation'}
 try {
   $packages=@(Get-AppxPackage -Name $name -ErrorAction Stop | Where-Object {$_.Name -ceq $name})
   if($packages.Count -eq 0){
     $result.status='not_installed';$result.message='Package not installed for current user.'
   } elseif(@($packages | Where-Object {$_.NonRemovable -or $_.IsFramework -or $_.IsResourcePackage -or $_.IsBundle}).Count -gt 0) {
     $result.status='protected';$result.message='Windows package type is protected; nothing was removed.'
   } else {
     foreach($pkg in $packages) {
       $full=[string]$pkg.PackageFullName
       if([string]::IsNullOrWhiteSpace($full)){throw 'No full package name'}
       Remove-AppxPackage -Package $full -Confirm:$false -ErrorAction Stop
     }
     $left=@(Get-AppxPackage -Name $name -ErrorAction Stop | Where-Object {$_.Name -ceq $name})
     if($left.Count -eq 0){
       $result.status='removed';$result.message='Windows confirms the app package is no longer registered for this user.'
     }else{
       $result.status='failed';$result.message='AppX package is still registered for this user.'
     }
   }
 } catch {
   $result.status='failed';$result.message='Windows could not remove or verify this package. Close the app or check permissions.'
 }
 $results+= [pscustomobject]$result
}
[ordered]@{results=@($results)} | ConvertTo-Json -Depth 4 -Compress
`

func removeWindowsOptionalApps(ctx context.Context, apps []optionalApp) (appRemovalReceipt, error) {
	ids := removalIDs(apps)
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", removeOptionalAppScript)
	// Passed into a fixed script after strict Go and PowerShell allowlisting;
	// no user-supplied command line/PowerShell source is executed.
	cmd.Env = append(os.Environ(), "SAWCHA_REMOVAL_SELECTION="+ids)
	raw, err := cmd.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return appRemovalReceipt{}, errors.New("removal timed out; some selected packages may already have been removed. Rescan before retrying")
		}
		return appRemovalReceipt{}, fmt.Errorf("PowerShell AppX removal failed; rescan Windows apps before retrying")
	}
	if len(raw) > 128*1024 {
		return appRemovalReceipt{}, errors.New("unexpectedly large Windows removal result")
	}
	var parsed struct {
		Results []appRemovalResult `json:"results"`
	}
	content := strings.TrimSpace(strings.TrimPrefix(string(raw), "\ufeff"))
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return appRemovalReceipt{}, errors.New("invalid Windows removal response; rescan to verify state")
	}
	if len(parsed.Results) != len(apps) {
		return appRemovalReceipt{}, errors.New("incomplete Windows removal response; rescan to verify state")
	}
	for i, result := range parsed.Results {
		if result.ID != apps[i].ID {
			return appRemovalReceipt{}, errors.New("Windows removal response did not match selected package IDs")
		}
		switch result.Status {
		case "removed", "not_installed", "protected", "failed":
		default:
			return appRemovalReceipt{}, errors.New("unexpected removal result status")
		}
		if len(result.Message) > 512 {
			return appRemovalReceipt{}, errors.New("unexpected removal message length")
		}
	}
	return appRemovalReceipt{Results: parsed.Results}, nil
}
