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

// The Scunthorpe half of the rule: a real surname is not profanity.
//
// This is the case the old flat substring list got wrong, and it got it wrong
// silently — nobody called Dickson ever reported it, because the signup just
// said no and they went away.
func TestARealNameIsNotProfanity(t *testing.T) {
	allowed := []string{
		"dickson", "dickens", "hancock", "woodcock", "cockburn",
		"spicer", "chinkara", "raccoon", "shitake", "analyst",
		"scunthorpe", "titsworth", "cockerill",
	}
	for _, name := range allowed {
		if got := ValidateUsername(name); got != "" {
			t.Errorf("ValidateUsername(%q) = %q, want allowed", name, got)
		}
	}
}

// The other half: the word itself, however it is spelled around.
func TestTheWordIsRefusedHoweverItIsSpelled(t *testing.T) {
	refused := []string{
		"dick_head",   // underscore is a word boundary
		"shit_poster", // ditto
		"d1ck_head",   // digits read as letters
		"n1gger",      // leetspeak in a slur
		"n_i_g_g_e_r", // underscores inside a slur do not break it up
		"fuck1234",    // digits as padding
		"d1ck123",     // padding and substitution at once
		"pornbot",     // substring, wherever it sits
		"xxfaggotxx",  // ditto
		"hitler",      // exact
		"admin",       // reserved
		"agent",       // reserved: takes delivery of agent@
		"postmaster",  // reserved: RFC 2142
	}
	for _, name := range refused {
		if ValidateUsername(name) == "" {
			t.Errorf("ValidateUsername(%q) returned no error, want refused", name)
		}
	}
}

// Create is the door, not the handler that happens to call it.
//
// The rule lived at two signup call sites and every other path wrote whatever
// it was handed. This asserts the rule is where it cannot be walked around.
func TestCreateRefusesAUsernameTheRuleForbids(t *testing.T) {
	homeDir(t, "create-validates")

	for _, bad := range []string{"3834", "abc", "Agent", "admin", "dick_head", "agent-name"} {
		if err := Create(&Account{ID: bad, Name: bad, Secret: "s"}); err == nil {
			t.Errorf("Create(%q) succeeded, want refused", bad)
			RemoveAccountForTest(bad)
		}
	}
	if err := Create(&Account{ID: "dickson", Name: "Dickson", Secret: "s"}); err != nil {
		t.Fatalf("Create(%q) refused a real name: %v", "dickson", err)
	}
}

// Claiming was the widest door: an account made from an inbound address could
// be renamed to anything, reserved names included.
func TestClaimRefusesAUsernameTheRuleForbids(t *testing.T) {
	homeDir(t, "claim-validates")

	SetAccountForTest(&Account{ID: "unclaimed_one", Unclaimed: true, Secret: "s"})
	t.Cleanup(func() { RemoveAccountForTest("unclaimed_one") })

	for _, bad := range []string{"admin", "agent", "postmaster", "n1gger", "abc", "3834"} {
		if err := Claim("unclaimed_one", bad, "a good password"); err == nil {
			t.Fatalf("Claim to %q succeeded, want refused", bad)
		}
	}
}
