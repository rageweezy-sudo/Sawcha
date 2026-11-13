# SAWCHA // OPTIMIZER — certificate-based Authenticode signing.
# Run only on YOUR Windows PC, as the Windows user who owns the code-signing certificate.
# Requires an existing, valid, trusted Code Signing certificate with its private key accessible
# through Windows Certificate Store. Does not create or export private keys.
[CmdletBinding()]
param(
  [string]$Thumbprint = '',
  [string]$ExecutablePath = (Join-Path $PSScriptRoot 'SAWCHA_OPTIMIZER.exe')
)
$ErrorActionPreference = 'Stop'
try {
    $file = (Resolve-Path -LiteralPath $ExecutablePath).Path
    if (-not $file.EndsWith('.exe', [System.StringComparison]::OrdinalIgnoreCase)) {
        throw 'Only an executable (.exe) can be signed by this script.'
    }
    $available = @(Get-ChildItem Cert:\CurrentUser\My -CodeSigningCert | Where-Object {
      $_.HasPrivateKey -and $_.NotBefore -le [DateTime]::UtcNow -and $_.NotAfter -ge [DateTime]::UtcNow
    })
    if ($Thumbprint) {
        $wanted = ($Thumbprint -replace '[^0-9a-fA-F]','').ToUpperInvariant()
        $matching = @($available | Where-Object { $_.Thumbprint.ToUpperInvariant() -eq $wanted })
    } else {
        $matching = $available
    }
    if ($matching.Count -ne 1) {
        Write-Host ''
        Write-Host 'Available personal Code Signing certificates:' -ForegroundColor Yellow
        foreach ($cert in $available) {
            Write-Host ('  ' + $cert.Subject + ' / Thumbprint: ' + $cert.Thumbprint + ' / Expires: ' + $cert.NotAfter.ToString('yyyy-MM-dd'))
        }
        if ($available.Count -eq 0) { throw 'No personal Code Signing certificate was found. You must obtain one from a trusted provider or enroll in an approved signing service.' }
        throw 'More than one certificate is available. Run again with -Thumbprint YOUR_CERTIFICATE_THUMBPRINT.'
    }
    $cert = $matching[0]
    Write-Host ('Using publisher certificate: ' + $cert.Subject) -ForegroundColor Cyan
    Write-Host ('Signing: ' + $file)
    $result = Set-AuthenticodeSignature -LiteralPath $file -Certificate $cert -HashAlgorithm SHA256 -TimestampServer 'http://timestamp.digicert.com'
    if ($result.Status -ne 'Valid') {
        throw ('The signature was not reported valid: ' + $result.Status + ' / ' + $result.StatusMessage)
    }
    $verified = Get-AuthenticodeSignature -LiteralPath $file
    if ($verified.Status -ne 'Valid') { throw ('Signature verification failed: ' + $verified.StatusMessage) }
    Write-Host 'Signature created and validated successfully.' -ForegroundColor Green
    Write-Host ('Publisher: ' + $verified.SignerCertificate.Subject)
    Write-Host ('SHA256: ' + (Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash)
} catch {
    Write-Host ('Could not sign executable: ' + $_.Exception.Message) -ForegroundColor Red
    exit 1
}
