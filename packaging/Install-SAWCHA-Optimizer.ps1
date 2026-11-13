$ErrorActionPreference = 'Stop'
try {
    $source = Split-Path -Parent $MyInvocation.MyCommand.Path
    $binary = Join-Path $source 'SAWCHA_OPTIMIZER.exe'
    if (-not (Test-Path -LiteralPath $binary -PathType Leaf)) {
        throw 'SAWCHA_OPTIMIZER.exe was not found beside the installer.'
    }
    $installDir = Join-Path $env:LOCALAPPDATA 'Programs\Sawcha Optimizer'
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
    Copy-Item -LiteralPath $binary -Destination (Join-Path $installDir 'SAWCHA_OPTIMIZER.exe') -Force
    $ico = Join-Path $source 'RAGE.ico'
    if (Test-Path -LiteralPath $ico) {
        Copy-Item -LiteralPath $ico -Destination (Join-Path $installDir 'RAGE.ico') -Force
    }
    $uninstaller = Join-Path $source 'Uninstall-SAWCHA-Optimizer.ps1'
    if (Test-Path -LiteralPath $uninstaller) {
        Copy-Item -LiteralPath $uninstaller -Destination (Join-Path $installDir 'Uninstall-SAWCHA-Optimizer.ps1') -Force
    }
    $shortcuts = @()
    $startMenu = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs'
    if (-not (Test-Path $startMenu)) { New-Item -ItemType Directory -Force -Path $startMenu | Out-Null }
    $shortcuts += (Join-Path $startMenu 'SAWCHA OPTIMIZER.lnk')
    $desktop = [Environment]::GetFolderPath('Desktop')
    if ($desktop) { $shortcuts += (Join-Path $desktop 'SAWCHA OPTIMIZER.lnk') }
    $shell = New-Object -ComObject WScript.Shell
    foreach ($path in $shortcuts) {
        $shortcut = $shell.CreateShortcut($path)
        $shortcut.TargetPath = Join-Path $installDir 'SAWCHA_OPTIMIZER.exe'
        $shortcut.WorkingDirectory = $installDir
        $shortcut.Description = 'SAWCHA // OPTIMIZER - Built by sawcha'
        if (Test-Path (Join-Path $installDir 'RAGE.ico')) {
            $shortcut.IconLocation = (Join-Path $installDir 'RAGE.ico') + ',0'
        }
        $shortcut.Save()
    }
    Write-Host 'SAWCHA // OPTIMIZER 6.0 Native installed successfully.' -ForegroundColor Red
    Write-Host "Location: $installDir"
    Write-Host 'Shortcuts: Desktop and Start Menu.'
    exit 0
} catch {
    Write-Host ('Installer error: ' + $_.Exception.Message) -ForegroundColor Red
    exit 1
}
