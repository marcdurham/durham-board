package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/eshaffer321/monarchmoney-go/pkg/monarch"
)

const sessionFile = ".monarch_session"
const cacheTTL = 5 * time.Minute

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

type TemplateData struct {
	Accounts       []*monarch.Account
	Transactions   []*monarch.Transaction
	LastUpdated    time.Time
	Error          string
	RefreshSecs    int
	CategoryTotals []CategoryTotal
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

func startRefreshLoop(ctx context.Context, client *monarch.Client, cache *Cache) {
	ticker := time.NewTicker(cacheTTL)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := refreshCache(ctx, client, cache); err != nil {
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

func categoryTotals(txs []*monarch.Transaction) []CategoryTotal {
	catMap := map[string]float64{}
	for _, tx := range txs {
		if tx.Amount < 0 {
			name := "Uncategorized"
			if tx.Category != nil {
				name = tx.Category.Name
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

func handleDashboard(tmpl *template.Template, cache *Cache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cache.mu.RLock()
		data := TemplateData{
			Accounts:     cache.Accounts,
			Transactions: cache.Transactions,
			LastUpdated:  cache.LastUpdated,
			Error:        cache.Error,
			RefreshSecs:  300,
		}
		cache.mu.RUnlock()

		data.CategoryTotals = categoryTotals(data.Transactions)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("template error: %v", err)
		}
	}
}

func main() {
	ctx := context.Background()

	token := os.Getenv("MONARCH_TOKEN")
	email := os.Getenv("MONARCH_EMAIL")
	password := os.Getenv("MONARCH_PASSWORD")

	_, sessionErr := os.Stat(sessionFile)
	sessionExists := sessionErr == nil

	if token == "" && email == "" && !sessionExists {
		log.Fatal("No credentials found. Set MONARCH_TOKEN, or MONARCH_EMAIL+MONARCH_PASSWORD, or provide a .monarch_session file.")
	}

	var client *monarch.Client
	var err error

	if token != "" {
		client, err = monarch.NewClientWithToken(token)
	} else {
		client, err = monarch.NewClient(&monarch.ClientOptions{
			SessionFile: sessionFile,
			Timeout:     30 * time.Second,
		})
	}
	if err != nil {
		log.Fatalf("creating monarch client: %v", err)
	}

	if token == "" && !sessionExists {
		if email == "" || password == "" {
			log.Fatal("No session file found. Set MONARCH_EMAIL and MONARCH_PASSWORD to log in.")
		}
		log.Println("No session file found — logging in...")
		if err := client.Auth.LoginInteractive(ctx, email, password); err != nil {
			log.Fatalf("login failed: %v", err)
		}
		if err := client.Auth.SaveSession(sessionFile); err != nil {
			log.Printf("warning: could not save session: %v", err)
		} else {
			log.Printf("Session saved to %s", sessionFile)
		}
	}

	cache := &Cache{}
	log.Println("Loading initial data from Monarch Money...")
	if err := refreshCache(ctx, client, cache); err != nil {
		cache.mu.Lock()
		cache.Error = fmt.Sprintf("Initial data load failed: %v", err)
		cache.mu.Unlock()
		log.Printf("warning: initial cache fill failed: %v", err)
	} else {
		log.Printf("Loaded %d accounts and %d transactions", len(cache.Accounts), len(cache.Transactions))
	}

	startRefreshLoop(ctx, client, cache)

	tmpl, err := template.ParseFiles("templates/dashboard.html")
	if err != nil {
		log.Fatalf("parsing template: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleDashboard(tmpl, cache))

	log.Printf("Listening on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
