package auth

import "testing"

func TestValidateUsernameFormat(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantErr  bool
	}{
		{name: "valid minimum length", username: "abcd"},
		{name: "valid maximum length", username: "abcdefghijklmnopqrstuvwx"},
		{name: "valid with digits and underscores", username: "agent_007"},
		{name: "too short", username: "abc", wantErr: true},
		{name: "too long", username: "abcdefghijklmnopqrstuvwxy", wantErr: true},
		{name: "must start with letter", username: "1agent", wantErr: true},
		{name: "uppercase rejected", username: "Agent", wantErr: true},
		{name: "hyphen rejected", username: "agent-name", wantErr: true},
		{name: "non ascii rejected", username: "ågent", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateUsername(tt.username)
			if tt.wantErr && got == "" {
				t.Fatalf("ValidateUsername(%q) returned no error", tt.username)
			}
			if !tt.wantErr && got != "" {
				t.Fatalf("ValidateUsername(%q) = %q, want no error", tt.username, got)
			}
		})
	}
}

func TestValidateUsernameReservedAndBlockedNames(t *testing.T) {
	tests := []struct {
		name     string
		username string
		want     string
	}{
		{name: "reserved", username: "admin", want: "That username is reserved."},
		{name: "blocked substring", username: "pornbot", want: "That username is not allowed."},

		// A username becomes a mailbox, and these are addresses the instance
		// already sends from. An agent answers mail as agent@<domain> and
		// event invites come from no-reply@<domain>, so whoever held one of
		// these names would take delivery of replies meant for the instance —
		// starting with everything anyone sends back to their own agent.
		{name: "the address agents reply from", username: "agent",
			want: "That username is reserved."},
		{name: "the address invites come from", username: "noreply",
			want: "That username is reserved."},
		{name: "postmaster", username: "postmaster",
			want: "That username is reserved."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateUsername(tt.username); got != tt.want {
				t.Fatalf("ValidateUsername(%q) = %q, want %q", tt.username, got, tt.want)
			}
		})
	}
}
