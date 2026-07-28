# Durham Board

A personal finance dashboard that pulls data from [Monarch Money](https://monarchmoney.com) and displays accounts, transactions, spending by category (pie chart), and recurring vendor tracking.

## Features

- Account balances overview
- Recent transaction list (last 90 days by default)
- Spending by category pie chart (configurable time window)
- Recurring vendor tracking with budget vs. actual
- Auto-refreshing data (configurable interval)

## Requirements

- Go 1.21+
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

The app supports three authentication methods, tried in this order:

### Option 1 — API Token (recommended)

Set the `MONARCH_TOKEN` environment variable to your Monarch Money API token.

```bash
MONARCH_TOKEN=your_token_here go run main.go
```

### Option 2 — Session file

Place a `.monarch_session` file in the project root. This file is created automatically when you use email/password login (see Option 3). On subsequent runs the session file is reused, so you won't need to log in again.

### Option 3 — Email + password (first-time login)

Set `MONARCH_EMAIL` and `MONARCH_PASSWORD`. The app will log in, then save the session to `.monarch_session` for future runs.

```bash
MONARCH_EMAIL=you@example.com MONARCH_PASSWORD=secret go run main.go
```

If none of the above are configured the app exits with an error explaining what's missing.

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
| `MONARCH_TOKEN` | — | Monarch Money API token |
| `MONARCH_EMAIL` | — | Email for password-based login |
| `MONARCH_PASSWORD` | — | Password for password-based login |
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

- `.monarch_session` contains a live auth token — it is gitignored by default. Do not commit it.
- `durbo.json` may contain personal financial category names — also gitignored.
