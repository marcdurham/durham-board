package main

import (
	"errors"
	"strings"
	"testing"
)

func TestLoginFailedMessageShowsFullEmail(t *testing.T) {
	const email = "alice+test@example.com"
	for _, err := range []error{
		errors.New("request failed: status 404"),
		errors.New("connection refused"),
	} {
		msg := loginFailedMessage(email, err)
		if !strings.Contains(msg, email) {
			t.Errorf("message for %q does not contain full email %q: %s", err, email, msg)
		}
	}
	if msg := loginFailedMessage(email, errors.New("status 404")); !strings.Contains(msg, "invalid email or password") {
		t.Errorf("404 message should explain invalid credentials: %s", msg)
	}
}

func TestLoginFailedMessageExplainsCaptcha(t *testing.T) {
	msg := loginFailedMessage("a@x.com", errors.New("error: CAPTCHA_REQUIRED"))
	if !strings.Contains(msg, "durham-board account cookie a@x.com") {
		t.Errorf("CAPTCHA message should point to the cookie command: %s", msg)
	}
	for _, step := range []string{"Network tab", "Cookie header", "app.monarch.com", "api.monarch.com/graphql"} {
		if !strings.Contains(msg, step) {
			t.Errorf("CAPTCHA message should explain how to copy the cookie (missing %q): %s", step, msg)
		}
	}
}
