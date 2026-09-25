package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "turso.tech/database/tursogo"
)

const defaultDBPath = "durbo.db"

// Account is a saved Monarch login. Cookie is a browser session cookie
// ("sessionid=...; csrftoken=...") and is preferred when set, since Monarch's
// web app authenticates that way and often blocks scripted password logins.
// Token is the most recent token from a password login, reused across runs
// so the app doesn't log in every start.
type Account struct {
	Email    string
	Password string
	Token    string
	Cookie   string
}

// Store keeps Monarch accounts and tokens in a local Turso database, replacing
// the old .monarch_session file and letting the active account be switched.
type Store struct {
	db *sql.DB
}

var errAccountNotFound = errors.New("account not found")

func openStore(path string) (*Store, error) {
	db, err := sql.Open("turso", path)
	if err != nil {
		return nil, fmt.Errorf("opening database %s: %w", path, err)
	}
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS accounts (
			email      TEXT PRIMARY KEY,
			password   TEXT NOT NULL DEFAULT '',
			token      TEXT NOT NULL DEFAULT '',
			cookie     TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return nil, fmt.Errorf("creating schema in %s: %w", path, err)
		}
	}
	// Databases created before cookie auth lack the cookie column.
	if err := addColumnIfMissing(db, "accounts", "cookie", `TEXT NOT NULL DEFAULT ''`); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

// addColumnIfMissing adds a column, treating "already exists" as success.
// It deliberately doesn't check pragma_table_info first: in tursogo v0.7.2
// querying it makes later writes on the connection silently not persist.
func addColumnIfMissing(db *sql.DB, table, column, def string) error {
	_, err := db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, def))
	if err != nil && strings.Contains(err.Error(), "duplicate column") {
		return nil
	}
	return err
}

func (s *Store) Close() error { return s.db.Close() }

// SaveAccount adds an account or updates its password. A password change
// clears the stored token so the next run logs in with the new credentials.
func (s *Store) SaveAccount(email, password string) error {
	_, err := s.db.Exec(`INSERT INTO accounts (email, password, updated_at) VALUES (?, ?, datetime('now'))
		ON CONFLICT(email) DO UPDATE SET
			token = CASE WHEN accounts.password = excluded.password THEN accounts.token ELSE '' END,
			password = excluded.password,
			updated_at = excluded.updated_at`, email, password)
	return err
}

func (s *Store) SetToken(email, token string) error {
	res, err := s.db.Exec(`UPDATE accounts SET token = ?, updated_at = datetime('now') WHERE email = ?`, token, email)
	if err != nil {
		return err
	}
	return requireRow(res, email)
}

func (s *Store) SetCookie(email, cookie string) error {
	res, err := s.db.Exec(`UPDATE accounts SET cookie = ?, updated_at = datetime('now') WHERE email = ?`, cookie, email)
	if err != nil {
		return err
	}
	return requireRow(res, email)
}

func (s *Store) Account(email string) (*Account, error) {
	var a Account
	err := s.db.QueryRow(`SELECT email, password, token, cookie FROM accounts WHERE email = ?`, email).
		Scan(&a.Email, &a.Password, &a.Token, &a.Cookie)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", errAccountNotFound, email)
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) Accounts() ([]Account, error) {
	rows, err := s.db.Query(`SELECT email, password, token, cookie FROM accounts ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.Email, &a.Password, &a.Token, &a.Cookie); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) RemoveAccount(email string) error {
	res, err := s.db.Exec(`DELETE FROM accounts WHERE email = ?`, email)
	if err != nil {
		return err
	}
	if err := requireRow(res, email); err != nil {
		return err
	}
	if active, _ := s.ActiveEmail(); active == email {
		_, err = s.db.Exec(`DELETE FROM settings WHERE key = 'active_account'`)
	}
	return err
}

// SetActive makes email the account the dashboard uses.
func (s *Store) SetActive(email string) error {
	if _, err := s.Account(email); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES ('active_account', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, email)
	return err
}

// ActiveEmail returns the active account's email, or "" if none is set.
func (s *Store) ActiveEmail() (string, error) {
	var email string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = 'active_account'`).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return email, err
}

// ActiveAccount returns the active account. If none has been chosen but
// exactly one account is saved, that one is used. Returns nil if there is
// nothing to use.
func (s *Store) ActiveAccount() (*Account, error) {
	email, err := s.ActiveEmail()
	if err != nil {
		return nil, err
	}
	if email != "" {
		return s.Account(email)
	}
	accounts, err := s.Accounts()
	if err != nil {
		return nil, err
	}
	if len(accounts) == 1 {
		return &accounts[0], nil
	}
	return nil, nil
}

func requireRow(res sql.Result, email string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %s", errAccountNotFound, email)
	}
	return nil
}
