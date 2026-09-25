package main

import (
	"reflect"
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
