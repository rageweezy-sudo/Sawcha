# Check whether this build has a Windows-trusted Authenticode signature.
[CmdletBinding()]
param([string]$ExecutablePath = (Join-Path $PSScriptRoot 'SAWCHA_OPTIMIZER.exe'))
$ErrorActionPreference = 'Stop'
try {
  $result = Get-AuthenticodeSignature -LiteralPath $ExecutablePath
  Write-Host ('Signature status: ' + $result.Status)
  if ($result.SignerCertificate) { Write-Host ('Publisher: ' + $result.SignerCertificate.Subject) }
  Write-Host ('SHA256: ' + (Get-FileHash -LiteralPath $ExecutablePath -Algorithm SHA256).Hash)
  if ($result.Status -ne 'Valid') {
    Write-Host 'This build is NOT verified as signed by a trusted publisher.' -ForegroundColor Yellow
    exit 1
  }
  Write-Host 'Certificate signature verified.' -ForegroundColor Green
} catch {
  Write-Host ('Verification error: ' + $_.Exception.Message) -ForegroundColor Red
  exit 1
}
