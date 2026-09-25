package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestMissingCredentials(t *testing.T) {
	tests := []struct {
		name                   string
		token, email, password string
		sessionExists          bool
		want                   []string
	}{
		// Regression: email+password without a token used to be rejected
		// because MONARCH_TOKEN was always reported missing.
		{"email and password suffice", "", "a@b.com", "pw", false, nil},
		{"token suffices", "tok", "", "", false, nil},
		{"session file suffices", "", "", "", true, nil},
		{"nothing set", "", "", "", false, []string{"MONARCH_EMAIL", "MONARCH_PASSWORD"}},
		{"password missing", "", "a@b.com", "", false, []string{"MONARCH_PASSWORD"}},
		{"email missing", "", "", "pw", false, []string{"MONARCH_EMAIL"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := missingCredentials(tt.token, tt.email, tt.password, tt.sessionExists)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

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
