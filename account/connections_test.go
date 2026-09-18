package account

import (
	"mu/internal/auth"
	"testing"
)

func TestAppPasswordIsProtocolOnly(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    []string
		want bool
	}{
		{"mail", []string{"read", "write", "protocol:mail"}, true},
		{"chat", []string{"read", "write", "protocol:chat"}, true},
		{"both", []string{"read", "write", "protocol:mail", "protocol:chat"}, true},
		{"legacy", []string{"read", "write"}, false},
		{"all", []string{"all", "protocol:mail"}, false},
		{"api", []string{"read", "write", "protocol:mail", "api:agent"}, false},
		{"service", []string{"read", "service:mail"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := appPassword(&auth.Token{Permissions: tc.p}); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
