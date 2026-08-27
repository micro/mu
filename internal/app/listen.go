package app

import (
	"os"
	"strings"
)

// The port each listener answers on when nobody has said otherwise.
//
// Here rather than beside each listener because the status page has to know
// them too, and it kept its own copies — so a default changed in one place was
// reported wrong in the other, on the page whose whole job is saying what is
// running. internal/app may not import a service, and a service already imports
// this, so this is the only place both can read.
//
// High rather than standard, for the three that clash: 25, 143 and 587 are
// what a mail server on the box already holds, and a development instance that
// fought it would refuse to start. 5222 is XMPP's real port and nothing else
// wants it. The SSH door has no default at all — see SHELL_SSH_PORT, where the
// port it would want is the host's own sshd and taking it locks you out.
const (
	MailPort       = ":2525"
	IMAPPort       = ":1143"
	SubmissionPort = ":1587"
	XMPPPort       = ":5222"
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
