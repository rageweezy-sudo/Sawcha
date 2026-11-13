SAWCHA // OPTIMIZER — OFFICIAL WINDOWS CODE-SIGNING
================================================
The compiled EXE is not currently digitally signed. A Microsoft-trusted
publisher signature is NOT created by consent, a gamer tag, or a self-signed
certificate.

If you personally own a genuine trusted code-signing certificate with a
private key in your CurrentUser\My Windows Certificate Store:

1. Keep your certificate/private key secure on your Windows machine.
2. Run a PowerShell window in the app folder.
3. Run the included script:
   .\SIGN_WITH_CERTIFICATE.ps1
   or, if you have multiple certificates:
   .\SIGN_WITH_CERTIFICATE.ps1 -Thumbprint YOUR_CERT_THUMBPRINT
4. Confirm success. Then run:
   .\VERIFY_SIGNATURE.ps1

The script uses Set-AuthenticodeSignature with SHA-256 and a timestamp. It
checks for a valid cert in your certificate store. It does not forge a name,
export a private key or bypass Windows Defender. Signing reduces unknown-
publisher warnings, but does NOT guarantee Defender/SmartScreen will allow
an executable. Suspected false detections should be submitted to Microsoft
Security Intelligence for analysis, not worked around via exclusions.

Do not upload PFX/private key/password files to chat or distribute them in ZIPs.
