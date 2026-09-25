# Changelog

## 2026-09-24

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
