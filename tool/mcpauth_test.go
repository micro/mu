package tool

import (
	"context"
	"testing"

	"mu/internal/api"

	"mu/internal/service"
)

// mailProbe stands in for the mail service: the check derives "needs auth" from
// whether the service behind the tool is scoped, so it has to be registered.
type MailProbe struct{}

type MailReq struct{ X string }
type MailRsp struct{ Y string }

func (MailProbe) Inbox(_ context.Context, _ *MailReq, _ *MailRsp) error { return nil }

func registerScopedMail(t *testing.T) {
	t.Helper()
	if _, ok := service.SpecFor("mail"); ok {
		return
	}
	if err := service.Register(service.Spec{
		Name: "mail", Handler: new(MailProbe), Scoped: true,
		// The tool exists because the Endpoint does. A Spec with no endpoints
		// derives nothing, which is what made this test read as "mail_inbox
		// does not challenge" when the real answer was that there was no
		// mail_inbox.
		Endpoints: map[string]service.Endpoint{
			"Inbox": {Doc: "List the account's most recent messages"},
		},
	}); err != nil {
		t.Fatalf("register mail: %v", err)
	}
}

// An MCP client discovers OAuth from a 401 naming the resource metadata. The
// HTTP layer can only send that challenge if it knows the named tool needs
// auth, which is what this answers.
func TestMCPToolNeedsAuthIdentifiesScopedTools(t *testing.T) {
	registerScopedMail(t)
	// The tools come from the Spec, so they have to be derived before the
	// protocol can be asked anything about them.
	DeriveTools()

	call := func(name string) []byte {
		return []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + name + `"}}`)
	}

	// Auth-bound and account-only tools must challenge.
	for _, name := range []string{"mail_inbox", "mail_send"} {
		if !api.MCPToolNeedsAuth(call(name)) {
			t.Errorf("%s does not challenge; a client would never discover OAuth", name)
		}
	}
	// Public tools must not — a challenge here would put news behind an account.
	for _, name := range []string{"news_search", "quran", "blog_read"} {
		if api.MCPToolNeedsAuth(call(name)) {
			t.Errorf("%s challenges; public tools must stay anonymous", name)
		}
	}
	// Anything that is not a tools/call, or names nothing, must not challenge.
	for _, body := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
		`not json`,
	} {
		if api.MCPToolNeedsAuth([]byte(body)) {
			t.Errorf("%q challenged and should not have", body)
		}
	}
}
