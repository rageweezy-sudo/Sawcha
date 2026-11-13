# Basic + Plus billing design

**Basic:** Always free; hardware scan, game detection, safe presets, diagnostics remain available without a license key.

**Plus:** Proposed $1.99/month via Stripe-hosted Checkout. The *Windows application* makes only HTTPS requests to the separate Plus service. It cannot issue its own paid key, fake Stripe checkout state, or call private developer vault routes.

Stripe Billing -> signed webhook -> server-side Stripe subscription GET -> exact-price validation -> encrypted per-installation Plus license -> Windows app periodic verification -> Plus UI. Cancellation / failed or expired paid periods return to Basic. Checkout success redirects never by themselves activate Plus.

**Security boundary:** The developer vault, password, TOTP seeds, original manual license database, code-signing material, Stripe secret keys, customer databases, and backups do not belong in the customer build or GitHub. The public Go service code has no developer login endpoints. Operations staff may manage billing through the *private* Stripe Dashboard.

**This phase:** end-to-end integration implemented and locally tested with mocked Stripe API; Windows executable cross-compiled. A real hosted Stripe test purchase is NOT proven until a test Stripe price, secret, webhook endpoint, persistent Railway volume and URL have been configured and a full purchase is exercised. The existing private manual-key vault currently maintains independent licenses — manually issued keys from it are not valid in the new Stripe service unless a future explicit bridge is implemented.

Customer cancellation uses Stripe's official Billing Portal when configured. A valid Plus license and the original device are required to open the portal even if payment recently failed. If the installed app or key is lost, the developer needs a support/recovery process.

Keep this integration in test mode until security review, tax setup, service uptime and backups, and entitlement gate behavior are complete. Never promise universal FPS gains.

## Developer-issued complimentary Pro passes

Alongside paid Plus, the same access features can be granted free via a signed gift. The **owner-only** Windows tool creates an Ed25519 key pair, protects the private key with DPAPI and signs time-limited `SAWCHA-GIFT-V1` passes. Only the public verification key (`SAWCHA_GIFT_PUBLIC_KEY`) is provided to the hosted server. The server checks expiry, signature, first-use device ID and revocations; it does not expose a key issuance route. No Stripe transaction occurs. Short-lived signed revoke commands can invalidate passes. See `docs/COMPLIMENTARY_PRO.md`.

Production caution: Never place the actual owner key, existing gift keys, customer records or Stripe credentials in the public repository. A paid plan's current feature checks are not guaranteed tamper-proof in an open-source Windows client; high-value online features need server-side entitlement authorization.
