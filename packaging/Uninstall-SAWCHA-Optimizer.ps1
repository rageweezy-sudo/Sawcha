$ErrorActionPreference = 'Stop'
$installDir = Join-Path $env:LOCALAPPDATA 'Programs\Sawcha Optimizer'
$desktop = [Environment]::GetFolderPath('Desktop')
$startMenu = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs'
foreach ($link in @((Join-Path $startMenu 'SAWCHA OPTIMIZER.lnk'), (Join-Path $desktop 'SAWCHA OPTIMIZER.lnk'))) {
    if (Test-Path -LiteralPath $link) { Remove-Item -LiteralPath $link -Force }
}
Write-Host 'Please close SAWCHA OPTIMIZER before finishing the uninstall.'
Start-Sleep -Seconds 2
if (Test-Path -LiteralPath $installDir) { Remove-Item -LiteralPath $installDir -Recurse -Force }
Write-Host 'Application uninstalled. Personal settings and benchmark history were kept at:'
Write-Host (Join-Path $env:LOCALAPPDATA 'Sawcha\Optimizer')
