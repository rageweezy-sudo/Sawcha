@echo off
setlocal
powershell.exe -NoProfile -ExecutionPolicy RemoteSigned -File "%~dp0Install-SAWCHA-Optimizer.ps1"
if errorlevel 1 (
 echo.
 echo Installation was not completed. Check the message above. If PowerShell blocks the installer, run the portable SAWCHA_OPTIMIZER.exe directly instead.
 pause
 exit /b 1
)
echo.
echo Installation complete. Launch from the desktop or Start menu.
pause
