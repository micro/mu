package app

import "testing"

// Turning a listener off must not be the thing that breaks it.
//
// IMAP_PORT understood "off" and MAIL_PORT did not, so an operator who wanted
// the web server without an MTA got "listen tcp: lookup tcp/off: unknown port"
// from a process that then exited — the SMTP listener called log.Fatal on a
// bind failure, so it took the web server with it. Both read this now.
func TestListenAddr(t *testing.T) {
	for _, tc := range []struct {
		value string
		addr  string
		on    bool
	}{
		{"", ":2525", true},   // unset is the default
		{"25", ":25", true},   // a bare port is what somebody types
		{":25", ":25", true},  // and so is the address form
		{" 25 ", ":25", true}, // trimmed, because a .env file has spaces in it
		{"127.0.0.1:2525", "127.0.0.1:2525", true},
		{"off", "", false},
		{"OFF", "", false},
		{"none", "", false},
		{"false", "", false},
		{"0", "", false},
		{"no", "", false},
	} {
		t.Setenv("TEST_LISTEN_PORT", tc.value)
		addr, on := ListenAddr("TEST_LISTEN_PORT", ":2525")
		if addr != tc.addr || on != tc.on {
			t.Errorf("ListenAddr(%q) = (%q, %v), want (%q, %v)", tc.value, addr, on, tc.addr, tc.on)
		}
	}
}
