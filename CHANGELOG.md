# Changelog

## 2026-09-25

- Authenticate with a browser session cookie: Monarch's web app no longer sends an `Authorization: Token` header, so the previous copy-the-token instructions couldn't be followed. `account cookie <email>` saves the `Cookie` header (`sessionid=...; csrftoken=...`) per account; `account token` is removed
- Upgrade to `github.com/eshaffer321/monarch-go/v2` v2.1.0 (cookie support, `api.monarch.com` endpoint)
- CAPTCHA and expired-cookie errors give step-by-step instructions for copying the cookie from the browser's developer tools; a rejected cookie no longer triggers a password login attempt
- `account list` shows what is saved for each account (cookie, token, password)
- Existing `durbo.db` files are upgraded in place with a `cookie` column

## 2026-09-24

- CAPTCHA login error (and README) now give step-by-step instructions for copying the token from the browser's developer tools; `account token` accepts a pasted `Token ` prefix
- Store Monarch accounts (email, password) and auth tokens in a local Turso database (`durbo.db`, override with `DURBO_DB`) instead of a `.monarch_session` file
- Add `durham-board account add|list|use|token|remove` to manage saved accounts and switch the active one
- Saved tokens are reused across restarts; an expired token triggers an automatic re-login with the saved password and the new token is stored
- `MONARCH_EMAIL`/`MONARCH_PASSWORD`, when set, are saved to the database and made active; `MONARCH_TOKEN` still overrides for a single run
- Login failures caused by Monarch's CAPTCHA now explain how to save a browser token with `account token`
- Requires Go 1.24+ (Turso driver)
- Login logs and errors now show the full email address instead of a masked one, so typos in MONARCH_EMAIL are visible
- Fix: email/password login was always rejected with "No credentials found" when no MONARCH_TOKEN was set; a token, a session file, or email+password now each suffice
- Credential error now lists the three ways to authenticate; expired-session error says to delete/rename `.monarch_session` to log in again

## 2026-06-27

- Add README.md with setup, auth, configuration, and environment variable docs

## 2026-06-25

- Pie chart period defaults to last 90 days and is configurable via `?pie_period_last_days=N` query parameter
- Recurring vendor tile now shows found/configured counts and spent/budget totals (e.g. `3/5` vendors, `$120/$200`)
- Pie chart legend: dollar amount left of color box, category name to the right (HTML legend replaces built-in Chart.js legend)
- Add `DURBO_DATA_PULL_MINUTES` env var to configure cache refresh interval (default: 5 minutes)
- Add `durbo.json` config file supporting `pie_chart_exclude_categories` (excludes named categories from the pie chart) and `recurring_vendors` (list of vendors with monthly budgets)
- Add recurring vendors tile on the dashboard showing active vendor count and monthly budget total for the last 30 days
- Add dollar amounts to pie chart legend labels and format all currency amounts with thousands separators
