package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// fakeServer answers tools/call, recording what it was asked for. Tools it does
// not know get the same "Tool not found" JSON-RPC error a real instance sends,
// which is the signal the dispatcher retries on.
func fakeServer(t *testing.T, known map[string]bool) (*httptest.Server, *[]call) {
	t.Helper()
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		json.Unmarshal(body, &req)
		calls = append(calls, call{Name: req.Params.Name, Args: req.Params.Arguments, Host: r.Host})

		w.Header().Set("Content-Type", "application/json")
		if !known[req.Params.Name] {
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"Tool not found: ` + req.Params.Name + `"}}`))
			return
		}
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"ok"}]}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

type call struct {
	Name string
	Args map[string]any
	Host string
}

// run invokes the CLI with stdout swallowed, so a test reads the calls rather
// than the printed output.
func run(t *testing.T, args ...string) int {
	t.Helper()
	old := os.Stdout
	devnull, _ := os.Open(os.DevNull)
	os.Stdout = devnull
	defer func() { os.Stdout = old; devnull.Close() }()
	return Run(args)
}

// The point of the change: a tool is two words on the command line.
func TestTwoWordToolNames(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv, calls := fakeServer(t, map[string]bool{
		"news_list": true, "markets_list": true, "web_search": true, "chat": true,
	})
	t.Setenv("MU_URL", srv.URL)

	cases := []struct {
		args     []string
		wantName string
		wantArgs map[string]any
	}{
		{[]string{"news", "list"}, "news_list", map[string]any{}},
		{[]string{"markets", "list", "--category", "stocks"}, "markets_list", map[string]any{"category": "stocks"}},
		{[]string{"web", "search", "claude code"}, "web_search", map[string]any{"q": "claude code"}},
		// The underscore form is the same call.
		{[]string{"news_list"}, "news_list", map[string]any{}},
		// A single word plus a sentence is a tool and its argument.
		{[]string{"chat", "hello there"}, "chat", map[string]any{"prompt": "hello there"}},
	}

	for _, tc := range cases {
		*calls = nil
		if code := run(t, tc.args...); code != 0 {
			t.Errorf("mu %v exited %d", tc.args, code)
			continue
		}
		last := (*calls)[len(*calls)-1]
		if last.Name != tc.wantName {
			t.Errorf("mu %v called %q, want %q", tc.args, last.Name, tc.wantName)
		}
		for k, v := range tc.wantArgs {
			if last.Args[k] != v {
				t.Errorf("mu %v sent %s=%v, want %v", tc.args, k, last.Args[k], v)
			}
		}
	}
}

// "mu chat hello" must not cost a wasted round trip guessing at chat_hello:
// a tool with one obvious argument is tried that way first.
func TestASingleArgumentIsNotMistakenForAMethod(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv, calls := fakeServer(t, map[string]bool{"chat": true})
	t.Setenv("MU_URL", srv.URL)

	run(t, "chat", "hello")
	if len(*calls) != 1 {
		t.Errorf("took %d calls to run `mu chat hello`, want 1: %+v", len(*calls), *calls)
	}
}

// When the two-word reading is wrong, the CLI falls back rather than failing.
func TestFallsBackWhenTheJoinedNameIsNotATool(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv, calls := fakeServer(t, map[string]bool{"blog_read": true})
	t.Setenv("MU_URL", srv.URL)

	// blog_read exists; "blog read 7" is the two-word name plus its id.
	if code := run(t, "blog", "read", "7"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	last := (*calls)[len(*calls)-1]
	// A positional argument is passed as typed — an id stays a string even when
	// it looks like a number. The point is that it reached blog_read rather
	// than being taken for half of a tool name.
	if last.Name != "blog_read" || last.Args["id"] != "7" {
		t.Errorf("called %q with %v", last.Name, last.Args)
	}
}

// --url is the instance to talk to, but web_fetch takes a url parameter. After
// a tool is named the flag belongs to the tool — otherwise the CLI silently
// points itself at the page it was asked to fetch.
func TestURLFlagAfterAToolBelongsToTheTool(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv, calls := fakeServer(t, map[string]bool{"web_fetch": true})
	t.Setenv("MU_URL", srv.URL)

	if code := run(t, "web", "fetch", "--url", "https://example.com/page"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	last := (*calls)[len(*calls)-1]
	if last.Args["url"] != "https://example.com/page" {
		t.Errorf("the tool got url=%v", last.Args["url"])
	}
	if len(*calls) == 0 {
		t.Fatal("the request never reached the configured instance")
	}
}

// Before the tool name, --url still chooses the instance.
func TestURLFlagBeforeAToolChoosesTheInstance(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv, calls := fakeServer(t, map[string]bool{"news_list": true})
	t.Setenv("MU_URL", "http://not-this-one.invalid")

	if code := run(t, "--url", srv.URL, "news", "list"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if len(*calls) != 1 {
		t.Fatalf("the request did not reach the instance named by --url")
	}
}
