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
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mu/internal/api"
	"mu/internal/service"
	"mu/internal/tool"
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
		if !api.ToolDispatch(p) {
			t.Errorf("ToolDispatch(%q) = false — this path dispatches tools, so the "+
				"wallet check, the auth challenge and the payment gate all skip it", p)
		}
	}
	for _, p := range []string{"/news", "/home", "/apiv1/news/list", "/api"} {
		if api.ToolDispatch(p) {
			t.Errorf("ToolDispatch(%q) = true — an ordinary page would be handed a 402", p)
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

// A method that changes something needs a POST.
//
// GET is offered because a REST API where reads are GETs is what every client
// library expects. The cost of that is a URL that acts, and a URL that acts
// gets fired by a link, a prefetch, a crawler or an <img src>. So the two are
// separated: read with either, change with POST.
//
// This used to ask Destructive, which is a different question — whether the
// *model* may hold the tool — and the two were one flag. Everything that wrote
// but was safe to hand an agent fell through: notes_add went out as a GET with
// the note in the query string, and so did docs_write, files_put, blog_create,
// chat_send. Writes is the flag for this question now, and
// Changes is the two of them together.
func TestAMethodThatChangesSomethingIsNotAGET(t *testing.T) {
	registerAll(t)
	loadTools(t)

	var changing, destructive int
	for _, sp := range service.Specs() {
		for name, ep := range sp.Endpoints {
			if !ep.Writes && !ep.Destructive {
				continue
			}
			changing++
			if ep.Destructive {
				destructive++
			}
			tool := sp.Name + "_" + strings.ToLower(name)
			if !service.ChangingTool(tool) {
				t.Errorf("service.ChangingTool(%q) is false, so the REST door would "+
					"serve it on a GET", tool)
			}
		}
	}
	if destructive < 5 {
		t.Fatalf("only %d destructive endpoints — this scan is broken", destructive)
	}
	if changing <= destructive {
		t.Fatalf("%d endpoints change something and %d of them are destructive, so "+
			"nothing is relying on Writes — which is the half that was missing",
			changing, destructive)
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
	if d := strings.Index(src, "service.ChangingTool("); d < get || d > post {
		t.Error("internal/api/rest.go does not refuse a changing tool inside its GET " +
			"branch — an <img src> can fire files_delete, or write a note")
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

// The handler itself refuses the GET, not the middleware in front of it.
//
// Reading the source above proves the check is written in the GET branch; this
// proves it fires. The distinction matters because of what the door answers
// unauthenticated: a scoped service refuses first, so /api/v1/notes/add?title=x
// comes back 401 whether or not the verb rule exists, and the case that
// actually needs guarding is the opposite one — a signed-in person's browser
// following an <img src> with their session on it. RESTHandler is what runs
// once that middleware has said yes.
func TestTheHandlerItselfRefusesAWritingGET(t *testing.T) {
	registerAll(t)
	loadTools(t)

	for _, path := range []string{
		"/api/v1/notes/add?title=x&text=y",
		"/api/v1/docs/write?title=x&body=y",
		"/api/v1/files/put?name=x",
		"/api/v1/blog/create?title=x",
		"/api/v1/images/generate?prompt=x",
	} {
		w := httptest.NewRecorder()
		api.RESTHandler(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 405 {
			t.Errorf("GET %s answered %d, want 405 — a URL that writes", path, w.Code)
		}
		if got := w.Header().Get("Allow"); got != "POST" {
			t.Errorf("GET %s refused without Allow: POST (got %q)", path, got)
		}
	}

	// And a read is still a read, or the rule has eaten the API.
	for _, path := range []string{"/api/v1/notes/list", "/api/v1/news/list"} {
		w := httptest.NewRecorder()
		api.RESTHandler(w, httptest.NewRequest("GET", path, nil))
		if w.Code == 405 {
			t.Errorf("GET %s was refused as a write", path)
		}
	}
}
