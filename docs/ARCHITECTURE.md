# Architecture and trust boundaries

- **Customer app** (`src/optimizer/`): native Go Win32 UI. Uses an HTTP API bound to `127.0.0.1` only, guarded with a per-launch token. It discovers local games and hardware and invokes explicit Windows diagnostic tools.
- **Local data**: stored under `%LOCALAPPDATA%\Sawcha\Optimizer`; user-specific settings, history and credentials are not shipped in the public source tree.
- **Optional external sign-in**: Google Desktop OAuth client ID is configured by the end user; the interactive login happens on Google's website. The local app verifies returned identity information.
- **Optional AI**: remote troubleshooting is explicitly initiated by the user and uses a key they provide. Do not bundle your own private key in builds.
- **Private licensing platform**: outside this repository and outside the customer installer. A future public API should return only signed/limited entitlement decisions, with sensitive license management performed server-side.

**Current limitation:** The customer app has no connected Stripe payment integration or Plus activation; the private developer key vault exists as a separate unpublished project. Public source inspection and unit tests are not a replacement for independent security assessment.
