package cli

import (
	"reflect"
	"testing"
)

func TestParseToolFlags(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantArgs map[string]any
		wantPos  []string
	}{
		{
			name:     "no args",
			args:     nil,
			wantArgs: map[string]any{},
			wantPos:  nil,
		},
		{
			name: "space-separated string",
			args: []string{"--query", "ai safety"},
			wantArgs: map[string]any{
				"query": "ai safety",
			},
		},
		{
			name: "equals-separated string",
			args: []string{"--query=ai safety"},
			wantArgs: map[string]any{
				"query": "ai safety",
			},
		},
		{
			name: "integer coerced",
			args: []string{"--limit", "10"},
			wantArgs: map[string]any{
				"limit": int64(10),
			},
		},
		{
			name: "float coerced",
			args: []string{"--lat", "51.5", "--lon", "-0.12"},
			wantArgs: map[string]any{
				"lat": 51.5,
				"lon": -0.12,
			},
		},
		{
			name: "bool explicit",
			args: []string{"--pollen", "true"},
			wantArgs: map[string]any{
				"pollen": true,
			},
		},
		{
			name: "bare bool flag",
			args: []string{"--public"},
			wantArgs: map[string]any{
				"public": true,
			},
		},
		{
			name: "positional trailing",
			args: []string{"--query", "ai", "extra"},
			wantArgs: map[string]any{
				"query": "ai",
			},
			wantPos: []string{"extra"},
		},
		{
			name:     "only positional",
			args:     []string{"hello world"},
			wantArgs: map[string]any{},
			wantPos:  []string{"hello world"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, pos, err := parseToolFlags(tc.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.wantArgs) {
				t.Errorf("args mismatch:\n want %v\n  got %v", tc.wantArgs, got)
			}
			if !reflect.DeepEqual(pos, tc.wantPos) {
				t.Errorf("positional mismatch:\n want %v\n  got %v", tc.wantPos, pos)
			}
		})
	}
}

func TestCoerce(t *testing.T) {
	cases := []struct {
		in   string
		want any
	}{
		{"hello", "hello"},
		{"true", true},
		{"false", false},
		{"10", int64(10)},
		{"-5", int64(-5)},
		{"1.5", 1.5},
		{"51.5", 51.5},
	}
	for _, tc := range cases {
		got := coerce(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("coerce(%q) = %v (%T), want %v (%T)", tc.in, got, got, tc.want, tc.want)
		}
	}
}

func TestDefaultArgKey(t *testing.T) {
	cases := map[string]string{
		"apps_build":    "prompt",
		"news_search":   "query",
		"video_search":  "query",
		"web_search":    "q",
		"places_search": "q",
		"web_fetch":     "url",
		"blog_read":     "id",
		"apps_search":   "query",
		// An app is addressed by slug. This said "id", and every documented
		// `mu apps_read <slug>` failed with "apps_read requires slug".
		"apps_read": "slug",
	}
	for tool, want := range cases {
		got, ok := defaultArgKey(tool)
		if !ok {
			t.Errorf("defaultArgKey(%q) returned false, want %q", tool, want)
			continue
		}
		if got != want {
			t.Errorf("defaultArgKey(%q) = %q, want %q", tool, got, want)
		}
	}
	if _, ok := defaultArgKey("mail_send"); ok {
		t.Error("defaultArgKey(mail_send) should return false")
	}
	// Neither of these is a tool, and this table used to claim both took a
	// prompt. `chat` is the discussion-rooms service — its tools are chat_send,
	// chat_rooms and chat_messages, none of which has a prompt field — and
	// `agent` stopped being one when agent_ask was removed. A mapping onto a
	// tool that does not exist is a lookup that can only fail. Talking to an
	// agent from the command line is `mu ask`.
	for _, dead := range []string{"chat", "agent"} {
		if _, ok := defaultArgKey(dead); ok {
			t.Errorf("defaultArgKey(%q) still maps a positional argument onto a "+
				"tool that does not exist", dead)
		}
	}
}

// A tool name is two words in a shell — "mu news list" — and one identifier
// over the wire. The split has to be unambiguous about which words are a name
// and which are an argument.
func TestSplitCommand(t *testing.T) {
	cases := []struct {
		command string
		rest    []string
		want    string
		wantOK  bool
	}{
		{"news", []string{"list"}, "news_list", true},
		{"web", []string{"search", "claude code"}, "web_search", true},
		{"markets", []string{"list", "--category", "stocks"}, "markets_list", true},
		{"news", nil, "", false},                                 // one word is a whole name
		{"agent", []string{"what is the btc price?"}, "", false}, // a sentence is an argument
		{"blog", []string{"--id", "7"}, "", false},               // a flag is not a method
		{"apps", []string{"my-app"}, "", false},                  // a dash makes it a value
	}
	for _, tc := range cases {
		got, _, ok := splitCommand(tc.command, tc.rest)
		if ok != tc.wantOK || got != tc.want {
			t.Errorf("splitCommand(%q, %v) = %q,%v; want %q,%v",
				tc.command, tc.rest, got, ok, tc.want, tc.wantOK)
		}
	}
}

// The words after a tool are only worth retrying as arguments if they could be
// arguments — otherwise "mu news nope" complains about an unexpected argument
// when the real mistake is a tool that does not exist.
func TestCanTakeArgs(t *testing.T) {
	if !canTakeArgs("news_list", []string{"--limit", "5"}) {
		t.Error("flags are always acceptable")
	}
	if canTakeArgs("news", []string{"nope"}) {
		t.Error("news has no positional argument, so nope cannot be one")
	}
	if canTakeArgs("blog", []string{"read", "7"}) {
		t.Error("two bare words are not one positional argument")
	}
	// This used to assert that `chat` took a positional prompt. It is not a
	// tool — see TestDefaultArgKey — and no one-word tool is, because a tool
	// name is derived as service_method.
}
