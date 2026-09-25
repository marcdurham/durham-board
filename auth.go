package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/eshaffer321/monarch-go/v2/pkg/monarch"
	"golang.org/x/term"
)

// authenticator keeps the Monarch client logged in as the active account,
// persisting each new token to the store so restarts don't need a login.
type authenticator struct {
	mu      sync.Mutex
	store   *Store
	account *Account // nil when MONARCH_TOKEN overrides the stored account
	// login signs in and returns the new token. interactive allows prompting
	// on the terminal for MFA/OTP codes.
	login func(ctx context.Context, email, password string, interactive bool) (string, error)
}

func newAuthenticator(client *monarch.Client, store *Store, account *Account) *authenticator {
	return &authenticator{
		store:   store,
		account: account,
		login: func(ctx context.Context, email, password string, interactive bool) (string, error) {
			var err error
			if interactive {
				err = client.Auth.LoginInteractive(ctx, email, password)
			} else {
				err = client.Auth.Login(ctx, email, password)
			}
			if err != nil {
				return "", err
			}
			sess := client.GetSession()
			if sess == nil || sess.Token == "" {
				return "", errors.New("login succeeded but no token was returned")
			}
			return sess.Token, nil
		},
	}
}

// relogin signs in with the saved password and stores the new token.
func (a *authenticator) relogin(ctx context.Context, interactive bool) error {
	if a.account == nil || a.account.Password == "" {
		return errors.New("no saved password to log in with")
	}
	log.Printf("Logging in to Monarch as %s...", a.account.Email)
	token, err := a.login(ctx, a.account.Email, a.account.Password, interactive)
	if err != nil {
		return errors.New(loginFailedMessage(a.account.Email, err))
	}
	a.account.Token = token
	if err := a.store.SetToken(a.account.Email, token); err != nil {
		return fmt.Errorf("saving token: %w", err)
	}
	log.Printf("Saved new token for %s", a.account.Email)
	return nil
}

// do runs fn, and if it fails because the token is missing or expired, logs
// in again with the saved password and retries once. A rejected browser
// cookie can't be renewed here, so that error says how to save a new one.
func (a *authenticator) do(ctx context.Context, interactive bool, fn func() error) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	err := fn()
	if err == nil || !isAuthError(err) || a.account == nil {
		return err
	}
	if a.account.Cookie != "" {
		return fmt.Errorf("%w: the saved browser cookie for %s has expired or was rejected; save a fresh one with: durham-board account cookie %s", err, a.account.Email, a.account.Email)
	}
	if a.account.Password == "" {
		return err
	}
	log.Printf("Token rejected (%v), logging in again", err)
	if lerr := a.relogin(ctx, interactive); lerr != nil {
		return fmt.Errorf("%w (re-login failed: %v)", err, lerr)
	}
	return fn()
}

// isAuthError reports whether err means the token is missing, expired, or
// rejected. The library's internal sentinel errors aren't exported, so match
// on their text.
func isAuthError(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, s := range []string{"not authenticated", "session expired", "unauthorized", "status 401", "status 403"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

const accountUsage = `usage:
  durham-board account list               list saved accounts (* = active)
  durham-board account add <email>        save an account (prompts for password) and make it active
  durham-board account cookie <email>     save a browser session cookie (prompts) for an account and make it active
  durham-board account use <email>        switch the dashboard to a saved account
  durham-board account remove <email>     delete a saved account and its token`

// runAccountCmd handles the "account" subcommand for managing saved logins.
func runAccountCmd(store *Store, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New(accountUsage)
	}
	needEmail := func() (string, error) {
		if len(args) != 2 || args[1] == "" {
			return "", fmt.Errorf("%s needs an email\n%s", args[0], accountUsage)
		}
		return args[1], nil
	}
	switch args[0] {
	case "list", "ls":
		accounts, err := store.Accounts()
		if err != nil {
			return err
		}
		if len(accounts) == 0 {
			fmt.Fprintln(stdout, "No saved accounts. Add one with: durham-board account add <email>")
			return nil
		}
		active, err := store.ActiveAccount()
		if err != nil {
			return err
		}
		for _, a := range accounts {
			mark := " "
			if active != nil && active.Email == a.Email {
				mark = "*"
			}
			var saved []string
			if a.Cookie != "" {
				saved = append(saved, "cookie")
			}
			if a.Token != "" {
				saved = append(saved, "token")
			}
			if a.Password != "" {
				saved = append(saved, "password")
			}
			if len(saved) == 0 {
				saved = append(saved, "nothing saved")
			}
			fmt.Fprintf(stdout, "%s %s (%s)\n", mark, a.Email, strings.Join(saved, ", "))
		}
		return nil
	case "add":
		email, err := needEmail()
		if err != nil {
			return err
		}
		fmt.Fprint(stdout, "Monarch password: ")
		password, err := readSecret(stdin, stdout)
		if err != nil {
			return err
		}
		if password == "" {
			return errors.New("password must not be empty")
		}
		if err := store.SaveAccount(email, password); err != nil {
			return err
		}
		if err := store.SetActive(email); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Saved %s and made it the active account\n", email)
		return nil
	case "cookie":
		email, err := needEmail()
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, cookieInstructions(email))
		fmt.Fprint(stdout, "Monarch cookie: ")
		raw, err := readSecret(stdin, stdout)
		if err != nil {
			return err
		}
		cookie, err := parseCookie(raw)
		if err != nil {
			return err
		}
		if _, err := store.Account(email); errors.Is(err, errAccountNotFound) {
			if err := store.SaveAccount(email, ""); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if err := store.SetCookie(email, cookie); err != nil {
			return err
		}
		if err := store.SetActive(email); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Saved cookie for %s and made it the active account\n", email)
		return nil
	case "use":
		email, err := needEmail()
		if err != nil {
			return err
		}
		if err := store.SetActive(email); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Active account is now %s (restart the dashboard to switch)\n", email)
		return nil
	case "remove", "rm":
		email, err := needEmail()
		if err != nil {
			return err
		}
		if err := store.RemoveAccount(email); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Removed %s\n", email)
		return nil
	}
	return fmt.Errorf("unknown account command %q\n%s", args[0], accountUsage)
}

// cookieInstructions explains how to copy the session cookie that Monarch's
// web app uses from the browser.
func cookieInstructions(email string) string {
	return fmt.Sprintf(`To copy your Monarch session cookie from a browser:
  1. Log in at https://app.monarch.com in a desktop browser (tick "Stay signed in" if offered).
  2. Open developer tools (F12, or Cmd+Option+I on a Mac) and select the Network tab.
  3. Reload the page, type graphql in the filter box, and click any request to api.monarch.com/graphql.
  4. Under Request Headers, find the Cookie header and copy its whole value (it contains sessionid=...; csrftoken=...).
  5. Run: durham-board account cookie %s   and paste it at the prompt.`, email)
}

// parseCookie cleans up a pasted Cookie header, accepting a leading
// "Cookie:" as copied from some browsers, and checks it has a session.
func parseCookie(raw string) (string, error) {
	c := strings.TrimSpace(raw)
	if len(c) >= 7 && strings.EqualFold(c[:7], "cookie:") {
		c = strings.TrimSpace(c[7:])
	}
	if c == "" {
		return "", errors.New("cookie must not be empty")
	}
	if !strings.Contains(c, "sessionid=") {
		return "", errors.New("that doesn't look like the Monarch Cookie header: it has no sessionid=... part. Copy the whole Cookie request header value")
	}
	return c, nil
}

// readSecret reads without echo on a terminal, or reads one line from piped
// input.
func readSecret(stdin io.Reader, stdout io.Writer) (string, error) {
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(stdout)
		return string(b), err
	}
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && line != "") {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
