package test

// A second door is where permissions go missing.
//
// /api/v1/ exists so a program that is not an agent can call a known method
// without speaking a tool-calling protocol. The whole argument for adding it is
// that it inherits rather than reimplements: it turns a path into a tool name
// and calls the same ExecuteTool /mcp calls, so scope, account-only refusals,
// quota, identity and price are decided once.
//
// That argument is only true while nothing here answers for itself. These check
// the joins — that the name translation is exact, that the door middleware
// upstream of the mux treats both paths alike, and that a destructive method
// cannot be fired by a GET.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mu/internal/api"
	"mu/internal/service"
	"mu/tool"
)

// loadTools derives the tool set from the registered specs, the way the server
// does. Registering a service is not the same as deriving its tools, and a test
// that forgets the second half checks an empty catalogue and passes.
func loadTools(t *testing.T) {
	t.Helper()
	tool.Load(service.Specs())
}

func TestThePathIsTheToolName(t *testing.T) {
	for _, c := range []struct{ path, want string }{
		{"/api/v1/news/list", "news_list"},
		{"/api/v1/web/search", "web_search"},
		{"/api/v1/News/List", "news_list"},
		{"/api/v1/news/list/", "news_list"},

		// Not a call. Each of these would be a tool name with a hole in it, and
		// a door that guessed would dispatch something nobody asked for.
		{"/api/v1/", ""},
		{"/api/v1/news", ""},
		{"/api/v1/news/list/extra", ""},
		{"/api/v1//list", ""},
		{"/api/v1/news/", ""},
	} {
		if got := api.RESTToolName(c.path); got != c.want {
			t.Errorf("RESTToolName(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// Both doors are doors, and the middleware asks one question rather than
// naming a path twice.
func TestBothDoorsAreRecognised(t *testing.T) {
	for _, p := range []string{"/mcp", "/api/v1/news/list", "/api/v1/"} {
		if !api.ToolDoor(p) {
			t.Errorf("ToolDoor(%q) = false — this path dispatches tools, so the "+
				"wallet check, the auth challenge and the payment gate all skip it", p)
		}
	}
	for _, p := range []string{"/news", "/home", "/apiv1/news/list", "/api", "/a2a"} {
		if api.ToolDoor(p) {
			t.Errorf("ToolDoor(%q) = true — an ordinary page would be handed a 402", p)
		}
	}
}

// The REST door answers the identity and price questions from the same
// functions the MCP door does, for the same tool.
//
// Asked of real tools rather than of a fixture, because the point is that the
// two doors cannot come to different conclusions about mail_send.
func TestTheTwoDoorsAgreeAboutEveryTool(t *testing.T) {
	registerAll(t)
	loadTools(t)

	var checked, needAuth, priced int
	for _, sp := range service.Specs() {
		for name := range sp.Endpoints {
			tool := sp.Name + "_" + strings.ToLower(name)
			if _, ok := api.ToolByName(tool); !ok {
				continue
			}
			checked++

			// The REST path for this tool resolves back to this tool.
			path := "/api/v1/" + sp.Name + "/" + strings.ToLower(name)
			if got := api.RESTToolName(path); got != tool {
				t.Errorf("%s resolves to %q — a caller would reach a different tool "+
					"than the catalogue advertised", path, got)
			}
			if api.ToolNeedsAuth(tool) {
				needAuth++
			}
			if api.ToolWalletOp(tool) != "" {
				priced++
			}
		}
	}

	if checked < 50 {
		t.Fatalf("only %d tools checked — this scan is broken, not the code", checked)
	}
	// And the questions are discriminating rather than answering the same way
	// for everything, which would make the agreement above meaningless.
	if needAuth == 0 || needAuth == checked {
		t.Errorf("%d of %d tools need auth — the question is not discriminating", needAuth, checked)
	}
	if priced == 0 {
		t.Error("no tool has a wallet operation, so the payment gate is guarding nothing")
	}
}

// A destructive method needs a POST.
//
// GET is offered because a REST API where reads are GETs is what every client
// library expects. The cost of that is a URL that acts, and a URL that acts
// gets fired by a link, a prefetch, a crawler or an <img src>. So the two are
// separated: read with either, change with POST.
func TestADestructiveMethodIsNotAGET(t *testing.T) {
	registerAll(t)
	loadTools(t)

	var found int
	for _, sp := range service.Specs() {
		for name, ep := range sp.Endpoints {
			if !ep.Destructive {
				continue
			}
			found++
			tool := sp.Name + "_" + strings.ToLower(name)
			if !service.DestructiveTool(tool) {
				t.Errorf("service.DestructiveTool(%q) is false, so the REST door would "+
					"serve it on a GET", tool)
			}
		}
	}
	if found < 5 {
		t.Fatalf("only %d destructive endpoints — this scan is broken", found)
	}

	// And the handler asks. A door that had the list and did not consult it
	// would pass everything above.
	b, err := os.ReadFile(filepath.Join(at(""), "internal/api/rest.go"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	get := strings.Index(src, "case http.MethodGet:")
	post := strings.Index(src, "case http.MethodPost:")
	if get < 0 || post < 0 {
		t.Fatal("the REST handler no longer branches on method")
	}
	if d := strings.Index(src, "service.DestructiveTool("); d < get || d > post {
		t.Error("internal/api/rest.go does not refuse a destructive tool inside its GET " +
			"branch — an <img src> can fire files_delete")
	}
}

// The door has no opinions of its own: it hands off to ExecuteTool, which is
// what makes "it inherits rather than reimplements" a fact rather than a claim.
func TestTheRESTDoorDecidesNothingItself(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(at(""), "internal/api/rest.go"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	if !strings.Contains(src, "ExecuteTool(r, name, args)") {
		t.Error("internal/api/rest.go does not dispatch through ExecuteTool — every " +
			"scope check, account-only refusal, quota charge and identity binding " +
			"would then be this file's own, and would drift from /mcp's")
	}
	for _, own := range []string{
		"auth.GetSession",    // identity is ExecuteTool's job
		"quota.ConsumeQuota", // so is charging
		"quota.CheckQuota",
	} {
		if strings.Contains(src, own) {
			t.Errorf("internal/api/rest.go calls %s itself — that is the second auth "+
				"story and the second price table this door exists to not have", own)
		}
	}
}

// A path that is a door but names no tool asks nothing and charges nothing.
func TestTheCatalogueIsNotATool(t *testing.T) {
	if got := api.RequestTool("/api/v1/", nil); got != "" {
		t.Errorf("RequestTool on the catalogue = %q — listing what exists would be "+
			"answered with a payment demand", got)
	}
	if api.ToolNeedsAuth("") || api.ToolWalletOp("") != "" {
		t.Error("the empty tool name needs auth or has a price, so any request that " +
			"names no tool is challenged")
	}
}

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
		if got := api.RESTStatus(c.text, c.err); got != c.want {
			t.Errorf("%s: status %d, want %d", c.what, got, c.want)
		}
	}
}

// And the strings matched above are the ones ExecuteTool actually writes.
// Matching on prose is only safe while something notices the prose changing.
func TestTheRefusalsMatchedAreTheOnesWritten(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(at(""), "internal/api/mcp.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{
		`"Authentication required", true, err`,
		`"This tool requires an account", true,`,
	} {
		if !strings.Contains(string(b), phrase) {
			t.Errorf("internal/api/mcp.go no longer returns %s — RESTStatus still "+
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
		if got := api.Typed(c.raw, c.declared); got != c.want {
			t.Errorf("Typed(%q, %q) = %#v, want %#v", c.raw, c.declared, got, c.want)
		}
	}
}
