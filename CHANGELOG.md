# Changelog

## 2026-06-25
- Pie chart legend: dollar amount left of color box, category name to the right (HTML legend replaces built-in Chart.js legend)
- Add `DURBO_DATA_PULL_MINUTES` env var to configure cache refresh interval (default: 5 minutes)
- Add `durbo.json` config file supporting `pie_chart_exclude_categories` (excludes named categories from the pie chart) and `recurring_vendors` (list of vendors with monthly budgets)
- Add recurring vendors tile on the dashboard showing active vendor count and monthly budget total for the last 30 days
- Add dollar amounts to pie chart legend labels and format all currency amounts with thousands separators
