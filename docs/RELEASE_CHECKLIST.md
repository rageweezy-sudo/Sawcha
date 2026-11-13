# Sawcha release checklist

This repository includes the public customer app only. **Never** push the private developer license vault, issued keys, payment credentials, signing private keys, developer passwords or customer data.

1. Run `go test ./...` and `go vet ./...` from `src/optimizer`.
2. Build the unsigned Windows x64 executable from reviewed source.
3. Test the actual native UI, first-run scan, game discovery, guest login, Google OAuth, diagnostics, FPS capture, backups and restoration on Windows 10 and 11.
4. Test with an AMD CPU and NVIDIA GPU, including machines without NVIDIA command-line utilities.
5. Check that each system optimization is reversible and improves measured performance in representative runs.
6. Inspect Defender/SmartScreen warnings and investigate any real detections. Do not advise security exclusions.
7. Sign the final unmodified executable using your verified certificate, ideally through a secured signing service, then verify its signature and SHA-256 checksum.
8. Set up and test a **separate** hosted licensing server, Stripe Checkout and signed webhooks before advertising Plus. Use test mode first and never embed Stripe secret keys into the customer app.
9. Document exactly what is and is not working; do not claim CI proved Windows hardware compatibility.
