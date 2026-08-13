package cli

import "testing"

// The failure this guards against is not "the call was missed". It is that a
// missed call becomes an invented answer: the loop takes the reply for prose,
// never runs the tool, and the model — shown no result — makes the news up.
func TestToolCallIsFoundAfterProse(t *testing.T) {
	reply := "Let me grab the latest headlines for you!\n\n" +
		`{"tool":"news_list","args":{"limit":10}}`

	call, ok := parseToolCall(reply)
	if !ok {
		t.Fatal("a call preceded by prose was read as a final answer")
	}
	if call.Tool != "news_list" {
		t.Errorf("tool = %q, want news_list", call.Tool)
	}
	if call.Args["limit"] != float64(10) {
		t.Errorf("args = %v, want limit 10", call.Args)
	}
}

func TestToolCallShapes(t *testing.T) {
	cases := []struct {
		name  string
		reply string
		want  string
	}{
		{"bare", `{"tool":"news_list","args":{}}`, "news_list"},
		{"fenced", "```json\n{\"tool\":\"web_search\",\"args\":{\"query\":\"x\"}}\n```", "web_search"},
		{"trailing prose", `{"tool":"markets_list","args":{}}` + "\nfetching that now", "markets_list"},
		{"nested args", `{"tool":"a","args":{"b":{"c":1}}}`, "a"},
		{"brace inside a string", `{"tool":"web_search","args":{"query":"a {b} c"}}`, "web_search"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			call, ok := parseToolCall(c.reply)
			if !ok {
				t.Fatalf("no call found in %q", c.reply)
			}
			if call.Tool != c.want {
				t.Errorf("tool = %q, want %q", call.Tool, c.want)
			}
		})
	}
}

// Prose has to stay prose, or every answer mentioning a brace becomes a call.
func TestProseIsNotAToolCall(t *testing.T) {
	for _, reply := range []string{
		"Here are today's headlines: politics, tech and sport.",
		"The JSON format is {key: value} in most languages.",
		`{"note":"no tool key here"}`,
		`{"tool":"","args":{}}`,
		"An unclosed brace { should not hang or match",
	} {
		if call, ok := parseToolCall(reply); ok {
			t.Errorf("prose %q parsed as a call to %q", reply, call.Tool)
		}
	}
}

// Args are optional; a tool taking none must still run.
func TestMissingArgsBecomeEmpty(t *testing.T) {
	call, ok := parseToolCall(`{"tool":"news_list"}`)
	if !ok {
		t.Fatal("a call with no args was rejected")
	}
	if call.Args == nil {
		t.Error("args is nil; the caller would pass a null arguments object")
	}
}
