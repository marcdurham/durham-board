package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := openStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStoreAccountsAndSwitching(t *testing.T) {
	s := newTestStore(t)

	if a, err := s.ActiveAccount(); err != nil || a != nil {
		t.Fatalf("empty store: got %v, %v; want nil, nil", a, err)
	}

	if err := s.SaveAccount("a@x.com", "pw1"); err != nil {
		t.Fatal(err)
	}
	// A single saved account is used even without an explicit choice.
	if a, _ := s.ActiveAccount(); a == nil || a.Email != "a@x.com" {
		t.Fatalf("single account should be active, got %+v", a)
	}

	if err := s.SaveAccount("b@x.com", "pw2"); err != nil {
		t.Fatal(err)
	}
	if a, _ := s.ActiveAccount(); a != nil {
		t.Fatalf("two accounts and none chosen should be ambiguous, got %+v", a)
	}

	if err := s.SetActive("b@x.com"); err != nil {
		t.Fatal(err)
	}
	if a, _ := s.ActiveAccount(); a == nil || a.Email != "b@x.com" || a.Password != "pw2" {
		t.Fatalf("got %+v, want b@x.com", a)
	}
	if err := s.SetActive("a@x.com"); err != nil {
		t.Fatal(err)
	}
	if a, _ := s.ActiveAccount(); a.Email != "a@x.com" {
		t.Fatalf("switch failed, got %s", a.Email)
	}

	if err := s.SetActive("nobody@x.com"); !errors.Is(err, errAccountNotFound) {
		t.Fatalf("SetActive unknown: got %v", err)
	}

	if err := s.RemoveAccount("a@x.com"); err != nil {
		t.Fatal(err)
	}
	// Removing the active account falls back to the single remaining one.
	if a, _ := s.ActiveAccount(); a == nil || a.Email != "b@x.com" {
		t.Fatalf("after removing active, got %+v", a)
	}
}

func TestStoreTokenKeptUnlessPasswordChanges(t *testing.T) {
	s := newTestStore(t)
	s.SaveAccount("a@x.com", "pw")
	if err := s.SetToken("a@x.com", "tok"); err != nil {
		t.Fatal(err)
	}

	s.SaveAccount("a@x.com", "pw")
	if a, _ := s.Account("a@x.com"); a.Token != "tok" {
		t.Errorf("same password should keep token, got %q", a.Token)
	}

	s.SaveAccount("a@x.com", "new")
	if a, _ := s.Account("a@x.com"); a.Token != "" || a.Password != "new" {
		t.Errorf("password change should clear token, got %+v", a)
	}

	if err := s.SetToken("nobody@x.com", "t"); !errors.Is(err, errAccountNotFound) {
		t.Errorf("SetToken unknown: got %v", err)
	}
}

func TestStorePersistsAcrossOpens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.db")
	s, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	s.SaveAccount("a@x.com", "pw")
	s.SetToken("a@x.com", "tok")
	s.SetActive("a@x.com")
	s.Close()

	s, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, err := s.ActiveAccount()
	if err != nil || a == nil || a.Token != "tok" {
		t.Fatalf("got %+v, %v", a, err)
	}
}

func TestActiveAccountSavesEnvCredentials(t *testing.T) {
	s := newTestStore(t)
	s.SaveAccount("old@x.com", "pw")
	s.SetActive("old@x.com")

	a, err := activeAccount(s, "new@x.com", "secret")
	if err != nil || a == nil || a.Email != "new@x.com" || a.Password != "secret" {
		t.Fatalf("got %+v, %v", a, err)
	}
	// Next run without env vars still uses the saved account.
	a, _ = activeAccount(s, "", "")
	if a == nil || a.Email != "new@x.com" {
		t.Fatalf("env account not remembered, got %+v", a)
	}
}

func TestAuthenticatorRelogsInOnExpiredToken(t *testing.T) {
	s := newTestStore(t)
	s.SaveAccount("a@x.com", "pw")
	s.SetToken("a@x.com", "stale")
	acct, _ := s.Account("a@x.com")

	logins := 0
	auth := &authenticator{store: s, account: acct,
		login: func(_ context.Context, email, password string, _ bool) (string, error) {
			logins++
			if email != "a@x.com" || password != "pw" {
				t.Errorf("login with %s/%s", email, password)
			}
			return "fresh", nil
		}}

	calls := 0
	err := auth.do(context.Background(), false, func() error {
		calls++
		if calls == 1 {
			return errors.New("fetching accounts: session expired")
		}
		return nil
	})
	if err != nil || calls != 2 || logins != 1 {
		t.Fatalf("err=%v calls=%d logins=%d", err, calls, logins)
	}
	if a, _ := s.Account("a@x.com"); a.Token != "fresh" {
		t.Errorf("new token not saved, got %q", a.Token)
	}

	// Non-auth errors are returned without logging in.
	err = auth.do(context.Background(), false, func() error { return errors.New("connection refused") })
	if err == nil || logins != 1 {
		t.Errorf("non-auth error: err=%v logins=%d", err, logins)
	}
}

func TestAccountCommand(t *testing.T) {
	s := newTestStore(t)
	run := func(stdin string, args ...string) (string, error) {
		var out bytes.Buffer
		err := runAccountCmd(s, args, strings.NewReader(stdin), &out)
		return out.String(), err
	}

	if _, err := run("pw1\n", "add", "a@x.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := run("pw2\n", "add", "b@x.com"); err != nil {
		t.Fatal(err)
	}
	out, _ := run("", "list")
	if !strings.Contains(out, "* b@x.com") || !strings.Contains(out, "  a@x.com") {
		t.Errorf("list after add should mark b active:\n%s", out)
	}
	if a, _ := s.Account("b@x.com"); a.Password != "pw2" {
		t.Errorf("password not read from stdin, got %q", a.Password)
	}

	// Cookie-only account, the usual case since Monarch blocks scripted logins.
	if _, err := run("Cookie: sessionid=abc; csrftoken=def\n", "cookie", "t@x.com"); err != nil {
		t.Fatal(err)
	}
	if a, _ := s.ActiveAccount(); a == nil || a.Email != "t@x.com" || a.Cookie != "sessionid=abc; csrftoken=def" || a.Password != "" {
		t.Errorf("cookie command: got %+v", a)
	}
	// Setting a cookie on an existing account keeps its password.
	if _, err := run("sessionid=x; csrftoken=y\n", "cookie", "b@x.com"); err != nil {
		t.Fatal(err)
	}
	if a, _ := s.Account("b@x.com"); a.Cookie != "sessionid=x; csrftoken=y" || a.Password != "pw2" {
		t.Errorf("cookie on existing account: got %+v", a)
	}
	if out, _ := run("", "list"); !strings.Contains(out, "b@x.com (cookie, password)") {
		t.Errorf("list should show what is saved:\n%s", out)
	}
	// Pasting something other than the Cookie header (e.g. an old token) is rejected.
	if _, err := run("Token abc\n", "cookie", "t@x.com"); err == nil {
		t.Error("value without sessionid should be rejected")
	}

	if _, err := run("", "use", "a@x.com"); err != nil {
		t.Fatal(err)
	}
	if out, _ := run("", "list"); !strings.Contains(out, "* a@x.com") {
		t.Errorf("use did not switch:\n%s", out)
	}

	if _, err := run("\n", "add", "c@x.com"); err == nil {
		t.Error("empty password should be rejected")
	}
	if _, err := run("", "use"); err == nil {
		t.Error("use without email should fail")
	}
	if _, err := run("", "bogus"); err == nil {
		t.Error("unknown subcommand should fail")
	}
	if _, err := run("", "remove", "a@x.com"); err != nil {
		t.Fatal(err)
	}
	if out, _ := run("", "list"); strings.Contains(out, "a@x.com") {
		t.Errorf("remove did not delete:\n%s", out)
	}
}

// Regression: databases created before cookie auth have no cookie column and
// must be upgraded in place without losing saved accounts.
func TestOpenStoreMigratesOldSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("turso", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE accounts (email TEXT PRIMARY KEY, password TEXT NOT NULL DEFAULT '', token TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '')`,
		`INSERT INTO accounts (email, password, token) VALUES ('a@x.com', 'pw', 'tok')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	for i := 0; i < 2; i++ { // reopening an already-migrated DB must also work
		s, err := openStore(path)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		if err := s.SetCookie("a@x.com", "sessionid=1"); err != nil {
			t.Fatal(err)
		}
		a, err := s.Account("a@x.com")
		if err != nil || a.Password != "pw" || a.Token != "tok" || a.Cookie != "sessionid=1" {
			t.Fatalf("got %+v, %v", a, err)
		}
		s.Close()
	}
}

func TestAuthenticatorDoesNotReloginWithRejectedCookie(t *testing.T) {
	s := newTestStore(t)
	s.SaveAccount("a@x.com", "pw")
	s.SetCookie("a@x.com", "sessionid=old")
	acct, _ := s.Account("a@x.com")

	auth := &authenticator{store: s, account: acct,
		login: func(context.Context, string, string, bool) (string, error) {
			t.Error("should not attempt password login when a cookie is saved")
			return "", nil
		}}
	err := auth.do(context.Background(), false, func() error { return errors.New("not authenticated") })
	if err == nil || !strings.Contains(err.Error(), "durham-board account cookie a@x.com") {
		t.Errorf("error should say how to save a fresh cookie: %v", err)
	}
}

func TestParseCookie(t *testing.T) {
	for in, want := range map[string]string{
		"sessionid=a; csrftoken=b":            "sessionid=a; csrftoken=b",
		"  Cookie: sessionid=a; csrftoken=b ": "sessionid=a; csrftoken=b",
		"cookie:sessionid=a":                  "sessionid=a",
	} {
		if got, err := parseCookie(in); err != nil || got != want {
			t.Errorf("parseCookie(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "Cookie:", "Token abc", "csrftoken=b"} {
		if _, err := parseCookie(in); err == nil {
			t.Errorf("parseCookie(%q) should fail", in)
		}
	}
}
