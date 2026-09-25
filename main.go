package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eshaffer321/monarch-go/v2/pkg/monarch"
)

const defaultPullMinutes = 5

type Config struct {
	PieChartExcludeCategories []string          `json:"pie_chart_exclude_categories"`
	RecurringVendors          []RecurringVendor `json:"recurring_vendors"`
}

type RecurringVendor struct {
	Name          string  `json:"name"`
	MonthlyBudget float64 `json:"monthly_budget"`
}

type Cache struct {
	mu           sync.RWMutex
	Accounts     []*monarch.Account
	Transactions []*monarch.Transaction
	LastUpdated  time.Time
	Error        string
}

type CategoryTotal struct {
	Name  string
	Total float64
}

type RecurringVendorSummary struct {
	Count           int
	ConfiguredCount int
	MonthlyTotal    float64
	ConfiguredTotal float64
}

type TemplateData struct {
	Accounts               []*monarch.Account
	Transactions           []*monarch.Transaction
	LastUpdated            time.Time
	Error                  string
	RefreshSecs            int
	RefreshMins            int
	CategoryTotals         []CategoryTotal
	RecurringVendorSummary *RecurringVendorSummary
	PiePeriodDays          int
}

func loadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	defer f.Close()
	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

func pullInterval() time.Duration {
	if v := os.Getenv("DURBO_DATA_PULL_MINUTES"); v != "" {
		var mins int
		if _, err := fmt.Sscan(v, &mins); err == nil && mins > 0 {
			return time.Duration(mins) * time.Minute
		}
		log.Printf("warning: invalid DURBO_DATA_PULL_MINUTES=%q, using default %d min", v, defaultPullMinutes)
	}
	return defaultPullMinutes * time.Minute
}

func refreshCache(ctx context.Context, client *monarch.Client, cache *Cache) error {
	accounts, err := client.Accounts.List(ctx)
	if err != nil {
		return fmt.Errorf("fetching accounts: %w", err)
	}

	end := time.Now()
	start := end.AddDate(0, 0, -90)

	result, err := client.Transactions.Query().
		Between(start, end).
		Limit(200).
		Execute(ctx)
	if err != nil {
		return fmt.Errorf("fetching transactions: %w", err)
	}

	var visible []*monarch.Account
	for _, a := range accounts {
		if !a.HideFromList {
			visible = append(visible, a)
		}
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.Accounts = visible
	cache.Transactions = result.Transactions
	cache.LastUpdated = time.Now()
	cache.Error = ""
	return nil
}

func startRefreshLoop(ctx context.Context, client *monarch.Client, auth *authenticator, cache *Cache) {
	interval := pullInterval()
	log.Printf("Cache refresh interval: %v", interval)
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				err := auth.do(ctx, false, func() error { return refreshCache(ctx, client, cache) })
				if err != nil {
					cache.mu.Lock()
					cache.Error = err.Error()
					cache.mu.Unlock()
					log.Printf("cache refresh error: %v", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func categoryTotals(txs []*monarch.Transaction, exclude []string) []CategoryTotal {
	excludeSet := make(map[string]bool, len(exclude))
	for _, c := range exclude {
		excludeSet[strings.ToLower(c)] = true
	}

	catMap := map[string]float64{}
	for _, tx := range txs {
		if tx.Amount < 0 {
			name := "Uncategorized"
			if tx.Category != nil {
				name = tx.Category.Name
			}
			if excludeSet[strings.ToLower(name)] {
				continue
			}
			catMap[name] += math.Abs(tx.Amount)
		}
	}
	cats := make([]CategoryTotal, 0, len(catMap))
	for k, v := range catMap {
		cats = append(cats, CategoryTotal{Name: k, Total: v})
	}
	sort.Slice(cats, func(i, j int) bool {
		return cats[i].Total > cats[j].Total
	})
	// Group anything beyond top 12 into "Other"
	if len(cats) > 12 {
		var other float64
		for _, c := range cats[12:] {
			other += c.Total
		}
		cats = append(cats[:12], CategoryTotal{Name: "Other", Total: other})
	}
	return cats
}

// recurringVendorSummary returns a count and monthly total for recurring vendors
// found in the last 30 days of transactions.
func recurringVendorSummary(txs []*monarch.Transaction, vendors []RecurringVendor) *RecurringVendorSummary {
	if len(vendors) == 0 {
		return nil
	}
	vendorSet := make(map[string]float64, len(vendors))
	var configuredTotal float64
	for _, v := range vendors {
		vendorSet[strings.ToLower(v.Name)] = v.MonthlyBudget
		configuredTotal += v.MonthlyBudget
	}

	cutoff := time.Now().AddDate(0, -1, 0)
	seen := map[string]bool{}
	var monthlyTotal float64
	for _, tx := range txs {
		if tx.Amount >= 0 {
			continue
		}
		if tx.Date.Before(cutoff) {
			continue
		}
		if tx.Merchant == nil {
			continue
		}
		key := strings.ToLower(tx.Merchant.Name)
		if budget, ok := vendorSet[key]; ok && !seen[key] {
			seen[key] = true
			monthlyTotal += budget
		}
	}
	return &RecurringVendorSummary{
		Count:           len(seen),
		ConfiguredCount: len(vendors),
		MonthlyTotal:    monthlyTotal,
		ConfiguredTotal: configuredTotal,
	}
}

// loginFailedMessage explains a failed email/password login, including the
// full email so the user can spot a typo in MONARCH_EMAIL.
func loginFailedMessage(email string, err error) string {
	// Monarch returns 404 for invalid credentials (security measure).
	// Surface a clearer message so users don't think the API is down.
	if strings.Contains(err.Error(), "status 404") {
		return fmt.Sprintf("Login failed for %s: Monarch returned HTTP 404. This usually means invalid email or password (Monarch uses 404 instead of 401 as a security measure). Verify your MONARCH_EMAIL and MONARCH_PASSWORD.", email)
	}
	if strings.Contains(err.Error(), "CAPTCHA_REQUIRED") {
		return fmt.Sprintf("Login failed for %s: Monarch is requiring a CAPTCHA, which can't be solved here. Use your browser's session cookie instead.\n%s", email, cookieInstructions(email))
	}
	return fmt.Sprintf("Login failed for %s: %v", email, err)
}

func formatMoney(v float64) string {
	formatted := fmt.Sprintf("%.2f", v)
	parts := strings.Split(formatted, ".")
	intPart := parts[0]
	decPart := parts[1]

	// Handle negative sign
	negative := false
	if strings.HasPrefix(intPart, "-") {
		negative = true
		intPart = intPart[1:]
	}

	// Add commas from right to left
	runes := []rune(intPart)
	var result strings.Builder
	for i, r := range runes {
		if i > 0 && (len(runes)-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(r)
	}

	intPart = result.String()
	if negative {
		return "$-" + intPart + "." + decPart
	}
	return "$" + intPart + "." + decPart
}

func handleDashboard(tmpl *template.Template, cache *Cache, cfg *Config) http.HandlerFunc {
	refreshSecs := int(pullInterval().Seconds())
	return func(w http.ResponseWriter, r *http.Request) {
		cache.mu.RLock()
		data := TemplateData{
			Accounts:     cache.Accounts,
			Transactions: cache.Transactions,
			LastUpdated:  cache.LastUpdated,
			Error:        cache.Error,
			RefreshSecs:  refreshSecs,
			RefreshMins:  refreshSecs / 60,
		}
		cache.mu.RUnlock()

		pieDays := 90
		if v := r.URL.Query().Get("pie_period_last_days"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				pieDays = n
			}
		}
		data.PiePeriodDays = pieDays
		pieCutoff := time.Now().AddDate(0, 0, -pieDays)
		var pieTxs []*monarch.Transaction
		for _, tx := range data.Transactions {
			if !tx.Date.Before(pieCutoff) {
				pieTxs = append(pieTxs, tx)
			}
		}
		data.CategoryTotals = categoryTotals(pieTxs, cfg.PieChartExcludeCategories)
		data.RecurringVendorSummary = recurringVendorSummary(data.Transactions, cfg.RecurringVendors)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("template error: %v", err)
		}
	}
}

func dbPath() string {
	if p := os.Getenv("DURBO_DB"); p != "" {
		return p
	}
	return defaultDBPath
}

// activeAccount picks the account to log in as. MONARCH_EMAIL and
// MONARCH_PASSWORD, when both set, are saved to the store and made active so
// later runs don't need them.
func activeAccount(store *Store, email, password string) (*Account, error) {
	if email != "" && password != "" {
		if err := store.SaveAccount(email, password); err != nil {
			return nil, fmt.Errorf("saving account %s: %w", email, err)
		}
		if err := store.SetActive(email); err != nil {
			return nil, err
		}
	} else if email != "" || password != "" {
		log.Printf("warning: MONARCH_EMAIL and MONARCH_PASSWORD must both be set to save an account; ignoring")
	}
	return store.ActiveAccount()
}

func main() {
	ctx := context.Background()

	store, err := openStore(dbPath())
	if err != nil {
		log.Fatalf("%v", err)
	}
	defer store.Close()

	if len(os.Args) > 1 {
		if os.Args[1] != "account" {
			log.Fatalf("unknown command %q\n%s", os.Args[1], accountUsage)
		}
		if err := runAccountCmd(store, os.Args[2:], os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	cfg, err := loadConfig("durbo.json")
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	account, err := activeAccount(store, os.Getenv("MONARCH_EMAIL"), os.Getenv("MONARCH_PASSWORD"))
	if err != nil {
		log.Fatalf("loading account: %v", err)
	}
	token := os.Getenv("MONARCH_TOKEN")
	if token == "" && account == nil {
		log.Fatalf("No Monarch account configured in %s.\nDo one of:\n  1. Run: durham-board account cookie <email>   (saves your browser session cookie; most reliable)\n  2. Run: durham-board account add <email>   (saves email/password; Monarch often blocks this with a CAPTCHA)\n  3. Set MONARCH_EMAIL and MONARCH_PASSWORD once (they are saved to %s)\n  4. Set MONARCH_TOKEN\nIf several accounts are saved, pick one with: durham-board account use <email>", dbPath(), dbPath())
	}

	opts := &monarch.ClientOptions{Timeout: 30 * time.Second}
	if token != "" {
		log.Printf("Using MONARCH_TOKEN")
		account = nil
		opts.Token = token
	} else {
		log.Printf("Using account %s", account.Email)
		// The library prefers the cookie when both are set.
		opts.Cookie = account.Cookie
		opts.Token = account.Token
	}
	client, err := monarch.NewClient(opts)
	if err != nil {
		log.Fatalf("creating monarch client: %v", err)
	}
	auth := newAuthenticator(client, store, account)

	if account != nil && account.Cookie == "" && account.Token == "" {
		if err := auth.relogin(ctx, true); err != nil {
			log.Fatal(err)
		}
	}

	cache := &Cache{}
	// Probe authentication up front so a stale/invalid session fails loudly
	// here (with a clear message) instead of later as a confusing API error.
	// A stored token that has expired is replaced by logging in again.
	if err := auth.do(ctx, true, func() error { return refreshCache(ctx, client, cache) }); err != nil {
		hint := "If using MONARCH_TOKEN, it may be expired."
		if account != nil {
			hint = cookieInstructions(account.Email)
		}
		log.Fatalf("Loading data from Monarch failed: %v\n%s", err, hint)
	}
	log.Printf("Loaded %d accounts and %d transactions", len(cache.Accounts), len(cache.Transactions))

	startRefreshLoop(ctx, client, auth, cache)

	tmpl, err := template.New("dashboard.html").Funcs(template.FuncMap{
		"formatMoney": formatMoney,
	}).ParseFiles("templates/dashboard.html")
	if err != nil {
		log.Fatalf("parsing template: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleDashboard(tmpl, cache, cfg))
	mux.HandleFunc("/merchants", func(w http.ResponseWriter, r *http.Request) {
		cache.mu.RLock()
		txs := cache.Transactions
		cache.mu.RUnlock()
		seen := map[string]bool{}
		var names []string
		for _, tx := range txs {
			if tx.Merchant == nil {
				continue
			}
			if !seen[tx.Merchant.Name] {
				seen[tx.Merchant.Name] = true
				names = append(names, tx.Merchant.Name)
			}
		}
		sort.Strings(names)
		w.Header().Set("Content-Type", "text/plain")
		for _, n := range names {
			fmt.Fprintln(w, n)
		}
	})

	log.Printf("Listening on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
