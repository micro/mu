package inbox

// What a mail client is told, and that the inbox says how to use itself.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mu/internal/auth"
)

func TestTheImapPageSaysWhatAClientAsksFor(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")
	const who = "imap_reader"
	if err := auth.Create(&auth.Account{ID: who}); err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("creating the account: %v", err)
	}
	sess, err := auth.CreateSession(who)
	if err != nil {
		t.Fatalf("creating the session: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/inbox/imap", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	ImapHandler(rec, req)
	page := rec.Body.String()

	// A client's form has four boxes and every one of them has to be fillable
	// from this page, or it is an explanation rather than an answer.
	for _, want := range []string{"example.test", "993", who, "/token"} {
		if !strings.Contains(page, want) {
			t.Errorf("a mail client cannot be set up from this page: no %q", want)
		}
	}
	// The username is the account, not the display name. That distinction cost
	// every Google sign-in their IMAP access once — see service/mail/credentials.go.
	if strings.Contains(page, "<td><code>Imap Reader</code>") {
		t.Error("the page offers a display name as the username")
	}
}

// Signed out there is nothing on it that is true about anybody.
func TestTheImapPageNeedsAnAccount(t *testing.T) {
	rec := httptest.NewRecorder()
	ImapHandler(rec, httptest.NewRequest("GET", "/inbox/imap", nil))
	if rec.Code != http.StatusSeeOther {
		t.Errorf("a stranger got %d rather than the sign-in page", rec.Code)
	}
}

// The port somebody types is not the port the process is bound to.
//
// Nothing in this repo terminates TLS, so the listener is behind a proxy and
// 1143 is the wrong number to print. A privileged port is one an operator
// chose, and is printed as-is.
func TestTheAdvertisedPortIsTheOneToConnectTo(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")

	if _, port, secure := advertised("", ":1143", "993"); !strings.HasPrefix(port, "993") {
		t.Errorf("a development listener advertised %q (%s)", port, secure)
	} else if !strings.Contains(port, "1143") {
		t.Error("the local port is not named, so a self-hoster with no proxy is stuck")
	}

	if _, port, secure := advertised("", ":143", "993"); port != "143" || secure != "STARTTLS" {
		t.Errorf("a port an operator chose was overridden: %q (%s)", port, secure)
	}

	// And the operator's own answer beats both.
	host, port, _ := advertised("mail.elsewhere.test:993", ":1143", "993")
	if host != "mail.elsewhere.test" || port != "993" {
		t.Errorf("the override was ignored: %s:%s", host, port)
	}
}

// The way into a mail client exists somewhere.
//
// This has been asserted about three different places, and the place is not the
// point: it is that /inbox/imap can be reached at all. It is a served page, and
// a served page with nothing linking to it is a page nobody can find.
//
// It was a four-line "how to use this page" list above the filters; then a text
// link beside New at the top of the inbox; and it is a card on /account now,
// with the password and the passkeys and the phone — the things you set up once
// and forget. Each move was for the same reason, which is that the inbox is
// opened every day and this is used once in the life of an account.
//
// So the test looks where it lives rather than where it used to. inbox may not
// import account, so this greps the source: the alternative is asserting
// nothing, and the failure being guarded against is a link deleted with no
// replacement, which a grep catches exactly.
func TestTheMailClientPageIsReachable(t *testing.T) {
	if strings.Contains(addressBar("someone", ""), `href="/inbox/imap"`) {
		return // still on the inbox, which also satisfies the property
	}
	b, err := os.ReadFile(filepath.Join("..", "account", "pages.go"))
	if err != nil {
		t.Fatalf("cannot read the account page: %v", err)
	}
	if !strings.Contains(string(b), `"/inbox/imap"`) {
		t.Error("nothing on the inbox links to the mail-client settings and " +
			"nothing on /account does either, so the page is served and " +
			"unreachable")
	}
}
