# Durham Board

A personal finance dashboard, designed to be displayed on a kiosk-like monitor or TV, that pulls data from [Monarch Money](https://monarchmoney.com) and displays accounts, transactions, spending by category (pie chart), and recurring vendor tracking.

## Features

- Account balances overview
- Recent transaction list (last 90 days by default)
- Spending by category pie chart (configurable time window)
- Recurring vendor tracking with budget vs. actual
- Auto-refreshing data (configurable interval)

## Requirements

- Go 1.24+ (no CGO needed)
- A Monarch Money account

## Running

```bash
go run main.go
```

The server starts on port `8082` by default. Override with the `PORT` env var:

```bash
PORT=3000 go run main.go
```

Open [http://localhost:8082](http://localhost:8082) in your browser.

## Authentication

Monarch accounts and their auth tokens are stored in a local [Turso](https://turso.tech) database file, `durbo.db` (override the path with `DURBO_DB`). No session file is used. After the first successful login the token is saved and reused, and if it expires the app logs in again with the saved password and stores the new token — you don't have to log in by hand.

### Managing accounts

```bash
durham-board account add you@example.com     # prompts for password, makes it active
durham-board account list                    # * marks the active account
durham-board account use other@example.com   # switch accounts (restart the dashboard)
durham-board account token you@example.com   # save a token directly (see CAPTCHA below)
durham-board account remove other@example.com
```

With `go run`, use `go run . account add you@example.com`.

If only one account is saved it is used automatically; with several, pick one with `account use`.

### Environment variables (optional)

- `MONARCH_EMAIL` + `MONARCH_PASSWORD` — if both are set, the account is saved to `durbo.db` and made active, so you can drop them afterwards.
- `MONARCH_TOKEN` — use this token for this run instead of the stored account (not saved).

### CAPTCHA

Monarch sometimes requires a CAPTCHA for password logins, which the app can't solve. When that happens, save a token from your browser instead:

1. Log in at <https://app.monarchmoney.com> in a desktop browser (solve the CAPTCHA there).
2. Open developer tools (F12, or Cmd+Option+I on a Mac) and select the **Network** tab.
3. Reload the page, type `graphql` in the filter box, and click any request to `api.monarchmoney.com/graphql`.
4. Under **Request Headers**, find `Authorization: Token <long value>` and copy the long value after `Token `.
5. Run `durham-board account token you@example.com` and paste it at the prompt (pasting the `Token ` prefix too is fine).

## Configuration

Copy `durbo.example.json` to `durbo.json` and edit it:

```bash
cp durbo.example.json durbo.json
```

```json
{
  "pie_chart_exclude_categories": ["Transfer", "Cash & ATM", "Credit Card Payment"],
  "recurring_vendors": [
    { "name": "Whole Foods", "monthly_budget": 400.00 },
    { "name": "Amazon",      "monthly_budget": 50.00 }
  ]
}
```

| Field | Description |
|---|---|
| `pie_chart_exclude_categories` | Category names to omit from the spending pie chart (case-insensitive). Useful for transfers and payments that inflate spending totals. |
| `recurring_vendors` | List of expected recurring vendors and their monthly budgets. The dashboard tile shows how many have been seen in the last 30 days and the total budget vs. actual spend. |

`durbo.json` is optional — the app runs with no config file.

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8082` | HTTP port to listen on |
| `DURBO_DB` | `durbo.db` | Path to the local Turso database holding accounts and tokens |
| `MONARCH_TOKEN` | — | Monarch Money API token (overrides the stored account for this run) |
| `MONARCH_EMAIL` | — | Email to save to the database and make active (with `MONARCH_PASSWORD`) |
| `MONARCH_PASSWORD` | — | Password to save to the database (with `MONARCH_EMAIL`) |
| `DURBO_DATA_PULL_MINUTES` | `5` | How often (in minutes) to refresh data from Monarch Money |

## Endpoints

| Path | Description |
|---|---|
| `/` | Main dashboard |
| `/merchants` | Plain-text list of all merchant names seen in recent transactions (useful for configuring `recurring_vendors`) |

### Pie chart time window

Append `?pie_period_last_days=N` to the dashboard URL to change the pie chart period:

```
http://localhost:8082/?pie_period_last_days=30
```

## Building

```bash
go build -o durham-board .
./durham-board
```

## Security notes

- `durbo.db` contains your Monarch passwords and live auth tokens in plain text — it is gitignored by default. Do not commit or share it, and keep it on the local machine.
- `durbo.json` may contain personal financial category names — also gitignored.
