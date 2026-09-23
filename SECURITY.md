# Security Policy

## How your data is handled
- **Desktop app:**
  - It listens only on `127.0.0.1` and rejects requests addressed to any other host name.
  - Distributor API keys are stored in `%APPDATA%\EMI Filter Designer\settings.json` with user-only permissions.
  - Keys are only ever sent to `api.mouser.com` or `api.digikey.com`.
- **Web app:**
  - Everything runs in your browser.
  - API keys are kept in that browser's `localStorage`. They are only sent to Mouser or DigiKey, or to a proxy that you configure yourself.
  - Never enter your keys on a copy of the site you don't trust, and never use a public proxy.

## Supported versions
Only the latest release receives fixes.

## Reporting a vulnerability
Please **do not** open a public issue for security problems. Use GitHub's **"Report a vulnerability"** button (Security tab → Advisories), or contact the maintainer privately. You should get an answer within 7 days.
