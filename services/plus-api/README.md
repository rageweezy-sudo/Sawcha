# SAWCHA Plus — customer payment and license API

This **separate Go service** supports Stripe-hosted Checkout and a customer billing portal. It is designed for deployment as a standalone Railway service. It does **not** serve a developer dashboard, issue arbitrary admin keys, or store private credentials in GitHub.

## Verified customer API surface

| Route | Purpose |
|---|---|
| `GET /health` | Health/configuration status without secrets |
| `GET /` | Pricing landing page (no payment collection) |
| `POST /v1/checkout` | Create Stripe Checkout session using a validated email and installation ID |
| `POST /v1/checkout/status` | Claim an activated Plus key using a private purchase token and the original installation ID |
| `POST /v1/activate` and `/v1/verify` | Server-side check for a paid, active subscription |
| `POST /v1/portal` | Stripe Billing customer portal URL for the subscribed installation |
| `POST /stripe/webhook` | Stripe-signed subscription lifecycle synchronization |

The public API contains **no administrative license-issuance, revoke, audit, or developer sign-in routes**. Manual developer key issuance remains inside the *separate private* developer vault and does not automatically integrate with this billing database. Do not distribute the private vault.

## Test-mode setup

1. Create a **test-mode** recurring Stripe product called `SAWCHA Plus`, USD **$1.99 each month**. Verify your tax requirements first; the code does not guess your tax treatment. Copy the `price_...` ID.
2. In Railway, deploy this directory using the repository-root `Dockerfile`. Mount a **persistent private volume** at `/data`. Use one replica; the bundled JSON store is not multi-writer safe.
3. Set `SAWCHA_DATA_DIR=/data` and a **random, persistent 32-byte encryption key** (64 hex characters) in `SAWCHA_DATA_KEY`. Keep it in Railway encrypted environment variables and encrypted backup, NEVER GitHub. Do not rotate it without a data migration.
4. Set `SAWCHA_PUBLIC_BASE_URL=https://<your verified public service domain>`. Set `STRIPE_SECRET_KEY=sk_test_...` and `STRIPE_PLUS_PRICE_ID=price_...` on the hosting platform only.
5. In Stripe **test mode**, create a webhook endpoint `https://<domain>/stripe/webhook` for `checkout.session.completed`, `customer.subscription.created`, `customer.subscription.updated`, `customer.subscription.deleted`, `invoice.paid`, `invoice.payment_failed`. Set its `whsec_...` as `STRIPE_WEBHOOK_SECRET` in Railway.
6. Enable **customer portal** configuration in Stripe test mode. Billing-portal behavior depends on this setting.
7. In the native app's **Plus Membership** screen, enter the public HTTPS server URL and customer email, select **Subscribe**, complete the hosted checkout, return to the app, and select **Check purchase status**. The app stores a random installation ID and the claim/key in `%LOCALAPPDATA%\Sawcha\Optimizer\plus.json` for future verification. Keep this user-local file private.
8. Test a real **test-mode** recurring purchase, cancellation, failed payment, renewal, different-device rejection, and network failure *before* enabling live billing or advertising Plus.

A successfully loaded checkout page does **not** grant Plus. The server cross-checks the subscription through authenticated Stripe API calls after verifying webhook signatures and matching the exact configured price. Cancellation or expiration falls back to Basic.

### Run locally

```sh
cd services/plus-api
export SAWCHA_DATA_KEY="$(openssl rand -hex 32)"
export SAWCHA_DATA_DIR="$(mktemp -d)"
go test -race ./...
go run . # runs /health on localhost:8080; checkout is unavailable without Stripe config
```

### Deployment and security limitations

This is a **test-mode integration, not a fully audited production billing system**. For production, replace the single-file JSON store with transactional durable storage, validate deployment backups and restore, add per-account and abuse rate limiting at the edge, provide customer key recovery (e.g. verified email), a data retention/deletion policy, refund/dispute workflows, chargeback handling, and monitoring. The app's local key cache is protected by the OS user profile, not yet DPAPI encryption. Do not switch to live Stripe billing until these are addressed.

Production `sk_live_` keys are explicitly rejected by default. Live mode requires the server-only `SAWCHA_ENABLE_LIVE_BILLING=I_ACKNOWLEDGE_LIVE_CHARGES` override. NEVER put the Stripe private key or webhook secret into the Windows app or this public repository.
