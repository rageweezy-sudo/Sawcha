# Sawcha security policy

This is a **public** repository. Never commit, paste into issues, or attach:

- Stripe secret keys, webhook signing secrets, private API keys, Google OAuth client secrets, login passwords or session tokens.
- Issued Plus license keys, activation records, license database backups, private developer key vault code/data, personal customer information.
- Code-signing certificate files (PFX/P12/PEM), private keys, timestamp-service credentials, or Azure credentials.
- Local `%LOCALAPPDATA%` config, hardware report exports, user-specific diagnostics.

Report security issues using GitHub's **private vulnerability reporting** if available for this repository. If unavailable, avoid public disclosure of active secrets or exploit details and use a private communication channel established with the maintainer.

Maintain least-privilege behavior: customer builds must not expose administrative licensing operations, secrets must stay server-side, and any publicly hosted verification endpoint must authenticate requests and rate-limit abuse.

Digital signing does not bypass antivirus scanning. Do not instruct users to disable Microsoft Defender to install the software.

## Research module policy (v9)

The GPU Compute Lab only reports capabilities and links to upstream public research. It does **not** unlock firmware, patch the Windows boot chain, alter driver signatures, make hidden registry writes, bypass anti-cheat, or bundle third-party GPL kernel components. Only high-confidence, repeatably benchmarked changes should be presented as performance improvements.
