package email

// The rules that are the service rather than decoration.
//
// Sending is the one thing here that cannot be undone, costs money the instant
// it happens, and spends something shared — a domain's reputation, which no
// amount of topping up repairs. So the guards around it are the product, and
// they are worth holding down the way sms holds its own.

import (
	"strings"
	"testing"

	"mu/internal/settings"
)

func withSettings(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		old := settings.Get(k)
		key, val := k, v
		settings.Set(key, val)
		t.Cleanup(func() { settings.Set(key, old) })
	}
}

// TestTheSendingDomainNeverFallsBackToTheRootDomain — the whole reason this
// service exists is that agent mail must not share a reputation with the domain
// the website is on. A default would undo it silently, on the instance where
// somebody forgot to configure it rather than the one where they thought about
// it.
func TestTheSendingDomainNeverFallsBackToTheRootDomain(t *testing.T) {
	withSettings(t, map[string]string{
		"EMAIL_DOMAIN":     "",
		"MAIL_DOMAIN":      "micro.mu",
		"SENDGRID_API_KEY": "SG.test",
	})
	if d := Domain(); d != "" {
		t.Errorf("with no EMAIL_DOMAIN set, mail would be sent from %q", d)
	}
	if Configured() {
		t.Error("email reports itself configured with no sending domain, so a send " +
			"would go out from wherever the provider defaults to")
	}
}

// TestRepliesArePointedAtSomethingWithAnInbox — the sending domain is
// authenticated for sending and has no MX record. A reply to a From on it
// bounces, the person answering gets an error, and the sender never learns
// their message was answered.
func TestRepliesArePointedAtSomethingWithAnInbox(t *testing.T) {
	withSettings(t, map[string]string{
		"EMAIL_DOMAIN":       "email.micro.mu",
		"MAIL_DOMAIN":        "micro.mu",
		"EMAIL_REPLY_DOMAIN": "",
	})
	if got := ReplyDomain(); got != "micro.mu" {
		t.Errorf("replies default to %q, want the instance's mail domain", got)
	}
	if ReplyDomain() == Domain() {
		t.Error("replies are pointed at the sending domain, which has no inbox behind it")
	}
}

// TestAnOperatorCanRedirectReplies.
func TestAnOperatorCanRedirectReplies(t *testing.T) {
	withSettings(t, map[string]string{
		"EMAIL_DOMAIN":       "email.micro.mu",
		"MAIL_DOMAIN":        "micro.mu",
		"EMAIL_REPLY_DOMAIN": "inbox.example.com",
	})
	if got := ReplyDomain(); got != "inbox.example.com" {
		t.Errorf("EMAIL_REPLY_DOMAIN was ignored: replies go to %q", got)
	}
}

// TestZeroIsTheKillSwitch — an operator typing 0 into /admin/env to stop the
// mail must not be told fifty. app.EnvInt treats 0 as "not set", which is right
// for a size and wrong for a limit.
func TestZeroIsTheKillSwitch(t *testing.T) {
	withSettings(t, map[string]string{"EMAIL_DAILY_LIMIT": "0"})
	if n := DailyLimit(); n != 0 {
		t.Errorf("EMAIL_DAILY_LIMIT=0 gives a limit of %d, so sending carries on", n)
	}
	if n := LimitFor("anybody"); n != 0 {
		t.Errorf("with sending off, one account still gets %d", n)
	}
}

// TestABadLimitDoesNotOpenTheGates — a typo must not read as unlimited.
func TestABadLimitDoesNotOpenTheGates(t *testing.T) {
	for _, v := range []string{"fifty", "-3", " "} {
		withSettings(t, map[string]string{"EMAIL_DAILY_LIMIT": v})
		if n := DailyLimit(); n != 50 {
			t.Errorf("EMAIL_DAILY_LIMIT=%q gives %d, want the default", v, n)
		}
	}
}

// TestSendingRefusesWithoutADomain — before any provider call, so a
// misconfigured instance fails at the door rather than at SendGrid.
func TestSendingRefusesWithoutADomain(t *testing.T) {
	withSettings(t, map[string]string{"EMAIL_DOMAIN": "", "SENDGRID_API_KEY": ""})
	_, err := Send("someone", "a@example.com", "Hi", "there")
	if err == nil {
		t.Fatal("a send was attempted with nothing configured")
	}
	if !strings.Contains(err.Error(), "sending domain") {
		t.Errorf("the refusal does not say what is wrong: %v", err)
	}
}

// TestALocalAddressIsSentBackToMail — email is for what leaves the building,
// and a username with no domain on it is somebody here.
func TestALocalAddressIsSentBackToMail(t *testing.T) {
	withSettings(t, map[string]string{
		"EMAIL_DOMAIN": "email.micro.mu", "SENDGRID_API_KEY": "SG.test",
	})
	_, err := Send("someone", "asim", "Hi", "there")
	if err == nil {
		t.Fatal("a bare username was accepted as an email address")
	}
	if !strings.Contains(err.Error(), "mail_send") {
		t.Errorf("the refusal does not say where to go instead: %v", err)
	}
}

// TestTheAddressIsSafeToPutInAHeader — an account name is whatever somebody
// typed, and it ends up on the left of an @.
func TestTheAddressIsSafeToPutInAHeader(t *testing.T) {
	for in, want := range map[string]string{
		"asim":          "asim",
		"Asim Aslam":    "asimaslam",
		"a.b-c_d":       "a.b-c_d",
		"foo@bar.com":   "foobar.com",
		"réal wörld":    "ralwrld",
		"!!!":           "agent",
		"":              "agent",
		"Line\nBreak":   "linebreak",
		"a b\r\nBcc: x": "abbccx",
	} {
		if got := localPart(in); got != want {
			t.Errorf("localPart(%q) = %q, want %q — a header injection or an "+
				"unroutable address starts here", in, got, want)
		}
	}
}

// TestTheBodyIsEscapedIntoHTML — the HTML part is built from the plain one, so
// a body containing markup must not become markup.
func TestTheBodyIsEscapedIntoHTML(t *testing.T) {
	got := asHTML("hello <script>alert(1)</script>\nsecond line\n\nnew para")
	if strings.Contains(got, "<script>") {
		t.Errorf("a body containing a script tag was sent as one: %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("the body was not escaped: %s", got)
	}
	if !strings.Contains(got, "<br>") || strings.Count(got, "<p>") != 2 {
		t.Errorf("line and paragraph breaks were lost: %s", got)
	}
}
