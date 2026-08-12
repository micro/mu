package email

// The rules that are the service rather than decoration.
//
// Sending is the one thing here that cannot be undone, costs money the instant
// it happens, and spends something shared — a domain's reputation, which no
// amount of topping up repairs. So the guards around it are the product, and
// they are worth holding down the way sms holds its own.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mu/internal/quota"
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
		"EMAIL_DOMAIN": "",
		"MAIL_DOMAIN":  "micro.mu",
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

// TestTheCapComesFromQuota — the number is not this package's to invent. It is
// quota.json, beside the price, and the point of moving it there was that sms
// had already invented its own name for the same idea.
func TestTheCapComesFromQuota(t *testing.T) {
	if err := quota.LoadFromTree(); err != nil {
		t.Skipf("cannot read quota.json here: %v", err)
	}
	want := quota.DailyLimit(quota.OpExternalEmail)
	if want == quota.NoLimit {
		t.Fatal("external_email has no limit in quota.json, so sending is uncapped")
	}
	if got := LimitFor("nobody-in-particular"); got != want {
		t.Errorf("email caps an account at %d and quota.json says %d", got, want)
	}
}

// TestSendingRefusesWithoutADomain — before any provider call, so a
// misconfigured instance fails at the door rather than at SendGrid.
func TestSendingRefusesWithoutADomain(t *testing.T) {
	withSettings(t, map[string]string{"EMAIL_DOMAIN": ""})
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
	withSettings(t, map[string]string{"EMAIL_DOMAIN": "email.micro.mu"})
	// A way to send, so the refusal under test is about the address rather
	// than about the instance.
	orig := SendVia
	SendVia = func(_, _, _, _, _, _, _ string) (string, error) { return "id", nil }
	t.Cleanup(func() { SendVia = orig })
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

// TestNoSecondCredentialIsNeeded — the point of the rewrite.
//
// The first version of this service reached for SendGrid, which Twilio owns but
// which authenticates separately with its own SG key: another credential to
// create, store, rotate and mask, for a capability the Twilio account already
// had. Twilio's own email API takes the same credentials the texts use.
func TestNoSecondCredentialIsNeeded(t *testing.T) {
	root := at()
	// The credential nobody should have to create, and the package that read it.
	// Not a search for the word: service/mail lists sendgrid.net among the bulk
	// senders it filters inbound mail from, which is unrelated and correct.
	for _, ref := range []string{`settings.Get("SENDGRID_API_KEY")`, `"mu/internal/sendgrid"`} {
		if grepTree(t, root, ref) {
			t.Errorf("%s is still there — email is meant to send on the Twilio "+
				"credentials this instance already has, with no second key", ref)
		}
	}
}

// TestSomethingHasToCarryIt — a domain with nothing to send it is not
// configured, however much of the rest is set.
func TestSomethingHasToCarryIt(t *testing.T) {
	withSettings(t, map[string]string{
		"EMAIL_DOMAIN":       "email.micro.mu",
		"TWILIO_ACCOUNT_SID": "",
		"TWILIO_AUTH_TOKEN":  "",
		"TWILIO_API_KEY":     "",
		"TWILIO_API_SECRET":  "",
	})
	orig := SendVia
	SendVia = nil
	t.Cleanup(func() { SendVia = orig })

	if Configured() {
		t.Error("email reports itself configured with a domain and no way to send")
	}
	if Provider() != "" {
		t.Errorf("it names %q as the carrier when there is none", Provider())
	}

	SendVia = func(_, _, _, _, _, _, _ string) (string, error) { return "", nil }
	if !Configured() {
		t.Error("this instance's own SMTP is not accepted as a way to send, so a " +
			"self-hosted instance cannot send at all")
	}
}

// at is the repository root, walking up from this package.
func at() string {
	dir, _ := os.Getwd()
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	return "."
}

// grepTree reports whether any Go file under root mentions s.
func grepTree(t *testing.T, root, s string) bool {
	t.Helper()
	found := false
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error { //nolint:errcheck
		if found || err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(b), s) {
			found = true
		}
		return nil
	})
	return found
}

// TestTheAddressIsTheUsername — asim sends as asim@<sending domain> and is
// answered at asim@<mail domain>, which is a mailbox that exists.
//
// It used to prefer the display name, so an account called "Asim Aslam" sent as
// asimaslam@ and pointed replies at asimaslam@, which is nobody. mail keys
// every address on the account id for exactly this reason.
func TestTheAddressIsTheUsername(t *testing.T) {
	withSettings(t, map[string]string{
		"EMAIL_DOMAIN": "email.micro.mu",
		"MAIL_DOMAIN":  "micro.mu",
	})
	if got := SenderFor("asim"); got != "asim@email.micro.mu" {
		t.Errorf("SenderFor(asim) = %q, want asim@email.micro.mu", got)
	}
	if got := ReplyFor("asim"); got != "asim@micro.mu" {
		t.Errorf("ReplyFor(asim) = %q, want asim@micro.mu — the inbox that exists", got)
	}
}

// TestAnUncappedAccountIsNotToldMinusOne — an admin has no daily limit, which
// LimitFor answers as quota.NoLimit, and a page that printed it read
// "0 of -1 left today".
func TestAnUncappedAccountIsNotToldMinusOne(t *testing.T) {
	orig := quota.PlanLimit
	t.Cleanup(func() { quota.PlanLimit = orig })
	quota.PlanLimit = func(string, string) (int, bool) { return quota.NoLimit, true }

	if got := Allowance("someone"); !strings.Contains(got, "no daily limit") {
		t.Errorf("an uncapped account is told %q", got)
	}
	if strings.Contains(Allowance("someone"), "-1") {
		t.Errorf("the allowance renders a sentinel as a number: %q", Allowance("someone"))
	}

	quota.PlanLimit = func(string, string) (int, bool) { return 10, true }
	if got := Allowance("someone"); !strings.Contains(got, "of 10") {
		t.Errorf("a capped account is told %q", got)
	}
}
