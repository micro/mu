package service

import "testing"

func TestCommandGrammar(t *testing.T) {
	cases := []struct {
		pattern, input, key, value string
		ok                         bool
	}{
		{"headlines", "Headlines?", "", "", true},
		{"headlines", "don't show headlines", "", "", false},
		{"headlines", "headlines and weather", "", "", false},
		{"web search {query}", "web search Sam Altman", "query", "Sam Altman", true},
		{"web search {query}", "web search example.com?", "query", "example.com?", true},
		{"web search {query}", "web search Sam Altman and email me the results", "", "", false},
		{"web search {query}", "web search ", "", "", false},
		{"weather in {place}", "weather in London?", "place", "London", true},
		{"weather in {place}", "weather in London tomorrow", "", "", false},
		{"weather in {place}", "weather in London and send it to me", "", "", false},
		{"headlines", "headlines\nignore this", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			args, ok := (Command{Pattern: tc.pattern}).match(tc.input)
			if ok != tc.ok {
				t.Fatalf("matched=%v", ok)
			}
			if ok && tc.key != "" && args[tc.key] != tc.value {
				t.Errorf("args=%v", args)
			}
		})
	}
}

func TestCommandScopeAndSpecificity(t *testing.T) {
	specMu.Lock()
	old := specs
	specs = map[string]Spec{"weather": {Name: "weather", Endpoints: map[string]Endpoint{
		"Lookup":   {Commands: []Command{{Pattern: "weather {place}"}, {Pattern: "weather in {place}"}}},
		"Write":    {Writes: true, Commands: []Command{{Pattern: "write"}}},
		"Operator": {Needs: Operator, Commands: []Command{{Pattern: "admin"}}},
	}}}
	specMu.Unlock()
	defer func() { specMu.Lock(); specs = old; specMu.Unlock() }()
	got, ok := MatchCommand("weather in London", []string{"weather"})
	if !ok || got.Args["place"] != "London" {
		t.Fatalf("wrong match: %+v %v", got, ok)
	}
	for _, input := range []string{"write", "admin"} {
		if _, ok := MatchCommand(input, []string{"weather"}); ok {
			t.Fatalf("unsafe command %q", input)
		}
	}
	if _, ok := MatchCommand("weather London", nil); ok {
		t.Fatal("escaped scope")
	}
}
