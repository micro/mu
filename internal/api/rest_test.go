package api

// Turning a refusal into a status, and a query string into arguments.
//
// Both are the REST door's own work rather than something inherited from
// ExecuteTool, so both are tested here — in the package, against the
// unexported functions, rather than exporting them so a test elsewhere can
// reach them. An export that exists for a test is an API somebody will use.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// A refusal says which kind it is.
//
// ExecuteTool reports in prose because its other caller is a model. A door
// serving programs has to turn that back into a status a client can branch on:
// re-authenticate, give up, or fix the arguments. Mapping everything to 400
// tells a client its arguments were wrong when it was simply not signed in.
func TestARefusalCarriesAStatusAClientCanActOn(t *testing.T) {
	for _, c := range []struct {
		what string
		text string
		err  error
		want int
	}{
		{"a tool that does not exist", "", errors.New("unknown tool: nope_nope"), 404},
		{"no caller at all", "Authentication required", errors.New("no session"), 401},

		// The other spelling. A spec-derived tool dispatches and the service
		// refuses with auth's own lowercase error, so the text is not
		// ExecuteTool's. blog_delete answered 400 until this was matched too.
		{"a service refusing for no caller", "", errors.New("authentication required"), 401},

		// Not 401: the caller proved who they are with a wallet and that
		// identity is not enough here, so retrying the same way loops.
		{"a wallet on an account-only tool", "This tool requires an account",
			errors.New("mail_send requires an account, not a paid wallet"), 403},

		{"bad arguments", "weather_forecast requires lat", nil, 400},
		{"a service refusing for its own reasons", "no account found for that address", nil, 400},

		// Mentioning it is not being it. A longer sentence is a service
		// explaining something, and telling a client to re-authenticate over it
		// sends it round a loop it cannot leave.
		{"a sentence that mentions it", "this step needs authentication required earlier", nil, 400},
	} {
		if got := restStatus(c.text, c.err); got != c.want {
			t.Errorf("%s: status %d, want %d", c.what, got, c.want)
		}
	}
}

// And the strings matched above are the ones ExecuteTool actually writes.
// Matching on prose is only safe while something notices the prose changing.
func TestTheRefusalsMatchedAreTheOnesWritten(t *testing.T) {
	b, err := os.ReadFile("mcp.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{
		`"Authentication required", true, err`,
		`"This tool requires an account", true,`,
	} {
		if !strings.Contains(string(b), phrase) {
			t.Errorf("internal/api/mcp.go no longer returns %s — restStatus still "+
				"matches it, so that refusal now reaches a client as a 400 telling "+
				"it to fix its arguments", phrase)
		}
	}
}

// A query string is all strings; the handlers behind these tools are not.
//
// lat=51.5 has to reach the dispatcher as a number or it answers "cannot
// unmarshal string into Go struct field" — correct, and useless, because a URL
// cannot carry anything else. The types come off the tool's own declaration
// rather than the shape of the value, so an id or a postcode that happens to be
// all digits stays a string.
func TestQueryArgumentsAreTypedByTheToolNotGuessed(t *testing.T) {
	for _, c := range []struct {
		raw, declared string
		want          any
	}{
		{"51.5", "number", 51.5},
		{"-0.12", "number", -0.12},
		{"7", "integer", 7},
		{"true", "boolean", true},
		{"false", "boolean", false},

		// Declared a string, so it stays one however numeric it looks. Guessing
		// from the value would turn a postcode, an id or a version into a float
		// and lose the leading zero on the way.
		{"01234", "string", "01234"},
		{"51.5", "string", "51.5"},

		// Undeclared: passed through rather than guessed at.
		{"51.5", "", "51.5"},

		// Declared but unparseable: passed through so the handler can say why,
		// rather than silently becoming a zero.
		{"north", "number", "north"},
		{"maybe", "boolean", "maybe"},
	} {
		if got := typed(c.raw, c.declared); got != c.want {
			t.Errorf("typed(%q, %q) = %#v, want %#v", c.raw, c.declared, got, c.want)
		}
	}
}

// A call through the API door is counted as the API, once.
//
// toolSurface answered "agent" for everything that was not /mcp, so every REST
// call was filed as this instance's own assistant — an operator reading usage
// would have seen the agent apparently getting busy and no sign of the API at
// all. And serve.go excluded only /mcp from the per-path counter, so the same
// call was also counted a second time as a path.
func TestACallIsAttributedToTheDoorItArrivedAt(t *testing.T) {
	for _, c := range []struct{ path, want string }{
		{"/mcp", "mcp"},
		{RESTRoot + "/news/list", "api"},
		{RESTPrefix + "news/list", "api"},

		// Not a door: this is the agent running a tool on somebody's behalf,
		// where the request is whatever page they were on.
		{"/agent", "agent"},
		{"/home", "agent"},
	} {
		r := httptest.NewRequest(http.MethodGet, c.path, nil)
		if got := toolSurface(r); got != c.want {
			t.Errorf("toolSurface(%q) = %q, want %q", c.path, got, c.want)
		}
	}
	if got := toolSurface(nil); got != "agent" {
		t.Errorf("toolSurface(nil) = %q, want agent", got)
	}
}

// A cookie is not a credential a program should be leaning on here.
//
// A browser attaches a cookie whoever caused the request, so a page on another
// origin can make a signed-in visitor's browser POST anywhere. A header has to
// be put there by the program making the call, which is why one is proof of
// origin and the other is not.
//
// This matters more than it looks: auth.ValidCSRF allows a request that omits
// the token entirely, as a grace period for pages already served, so CSRF is
// not in practice enforced anywhere. A door opened today has no stale clients
// and uses auth.StrictCSRF instead.
func TestOnlyANonCookieCredentialSkipsTheCSRFCheck(t *testing.T) {
	for _, c := range []struct {
		what   string
		set    func(*http.Request)
		header bool
	}{
		{"nothing at all", func(r *http.Request) {}, false},
		{"a session cookie", func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: "session", Value: "whatever"})
		}, false},
		{"a bearer token", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer abc")
		}, true},
		{"the legacy token header", func(r *http.Request) {
			r.Header.Set(TokenHeader, "abc")
		}, true},
	} {
		r := httptest.NewRequest(http.MethodPost, RESTPrefix+"notes/add", nil)
		c.set(r)
		if got := headerCredential(r); got != c.header {
			t.Errorf("%s: headerCredential = %v, want %v", c.what, got, c.header)
		}
	}
}

// And the handler consults it, on POST only. A door that computed the answer
// and did not use it would pass the test above.
func TestTheDoorAsksBeforeItChangesAnything(t *testing.T) {
	b, err := os.ReadFile("rest.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	get := strings.Index(src, "case http.MethodGet:")
	post := strings.Index(src, "case http.MethodPost:")
	if get < 0 || post < 0 {
		t.Fatal("the handler no longer branches on method")
	}
	i := strings.Index(src, "auth.StrictCSRF(r)")
	if i < 0 {
		t.Fatal("the handler does not check CSRF at all — a page on another origin " +
			"can make a signed-in visitor POST here")
	}
	if i < post {
		t.Error("the CSRF check is not in the POST branch — either reads are being " +
			"refused, or writes are not being checked")
	}
	if strings.Contains(src, "auth.ValidCSRF(") {
		t.Error("the handler uses auth.ValidCSRF, which allows a request that supplies " +
			"no token at all — that is the grace period for pages already in the " +
			"wild, and this door has none")
	}
}
