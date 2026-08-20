package app

import (
	"os"
	"strings"
)

// ListenAddr reads a listener's port out of the environment.
//
// Two things every listener configured this way needs, written once because
// they were written twice and only one of them was right. IMAP understood
// IMAP_PORT=off and SMTP did not, so an operator who wanted the web server
// without an MTA set MAIL_PORT=off, and the string reached net.Listen as a port
// name: "listen tcp: lookup tcp/off: unknown port", from a process that then
// exited. Turning something off should not be the thing that breaks it.
//
// The bare port is accepted as well as :port, because that is what somebody
// types.
func ListenAddr(key, fallback string) (addr string, on bool) {
	value := strings.TrimSpace(os.Getenv(key))
	switch strings.ToLower(value) {
	case "off", "false", "0", "no", "none", "disabled":
		return "", false
	case "":
		value = fallback
	}
	if !strings.Contains(value, ":") {
		value = ":" + value
	}
	return value, true
}
