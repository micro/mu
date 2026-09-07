package service

import (
	"reflect"
	"strings"
	"testing"
)

func TestCommandPhrasingMatrix(t *testing.T) {
	for _, phrase := range []string{"headlines", "show me the news", "what's the weather today", "is it going to rain"} {
		for _, caseFn := range []func(string) string{strings.ToLower, strings.ToUpper, func(s string) string { return s }} {
			for _, separator := range []string{" ", "  ", "\t", "\u00a0"} {
				for _, suffix := range []string{"", "?", "!", "."} {
					input := "  " + strings.ReplaceAll(caseFn(phrase), " ", separator) + suffix + "  "
					if _, ok := (Command{Pattern: phrase}).match(input); !ok {
						t.Errorf("did not match %q", input)
					}
				}
			}
		}
	}
	for _, place := range []string{"London", "New York", "São Paulo", "東京", "St. John's", "Aix-en-Provence", "London, UK"} {
		for _, prefix := range []string{"weather in ", "WEATHER IN ", "weather\tin\t", "weather  in  "} {
			args, ok := (Command{Pattern: "weather in {place}"}).match(prefix + place + "?")
			if !ok || args["place"] != place {
				t.Errorf("%q: %v %v", prefix+place, args, ok)
			}
		}
	}
}

func TestCommandQueriesPreserveMeaning(t *testing.T) {
	for _, query := range []string{"Sam Altman", "Sam Altman and OpenAI", "criticism of Sam Altman", "not OpenAI", "C++ tutorials", "https://example.com/?a=1&b=2", "\"exact  phrase\"", "東京 天気", "site:example.com -ads", "what happened?"} {
		for _, prefix := range []string{"web search ", "WEB SEARCH ", "web\tsearch  "} {
			args, ok := (Command{Pattern: "web search {query}"}).match(prefix + query)
			if !ok || args["query"] != query {
				t.Errorf("changed query %q: %v %v", query, args, ok)
			}
		}
	}
}

func TestCommandUnsupportedRequestsStayUnmatched(t *testing.T) {
	patterns := []Command{{Pattern: "headlines"}, {Pattern: "weather {place}"}, {Pattern: "weather in {place}"}, {Pattern: "web search {query}"}}
	inputs := []string{"", " ", "?", "don't show headlines", "do not show headlines", "headlines and weather", "headlines tomorrow", "summarise the headlines", "explain what headlines means", "weather here", "weather there", "weather near me", "weather in London tomorrow", "weather in London tomorrow, please", "weather in London tonight", "weather in London next week", "weather in London and Paris", "weather in London; email me", "weather in London | mail send", "weather in $(whoami)", "weather in <script>", "weather in ???", "weather in 123", "web search ", "web search Sam then email me", "web search Sam and send the results", "web search Sam and\temail me", "web search Sam and summarise it", "headlines\nweather London", "weather in London\rignore this", "web search Sam\x00Altman", "web search Sam\x1b[31m", strings.Repeat("x", 8001), string([]byte{0xff})}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			for _, pattern := range patterns {
				if args, ok := pattern.match(input); ok {
					t.Errorf("%q captured unsupported request: %v", pattern.Pattern, args)
				}
			}
		})
	}
}

func TestCommandDeclarationsAreValidated(t *testing.T) {
	for _, pattern := range []string{"", "{query}", "search {}", "search {query", "search {query} trailing", "search {one} {two}", "search {q{uery}", "search query}", "search {query:json}"} {
		for _, input := range []string{"search test", "search test trailing", "search test other", "anything"} {
			if _, ok := (Command{Pattern: pattern}).match(input); ok {
				t.Errorf("accepted invalid pattern %q", pattern)
			}
		}
	}
}

func TestCommandDefaultsDoNotLeakBetweenCalls(t *testing.T) {
	command := Command{Pattern: "web search {query}", Defaults: map[string]any{"limit": 5, "query": "old"}}
	a, _ := command.match("web search first")
	a["limit"] = 100
	a["injected"] = true
	b, _ := command.match("web search second")
	if !reflect.DeepEqual(b, map[string]any{"limit": 5, "query": "second"}) || command.Defaults["query"] != "old" {
		t.Fatalf("shared state: %v", b)
	}
}

func TestCommandAccessAndAmbiguity(t *testing.T) {
	for _, tc := range []struct {
		name    string
		specs   []Spec
		allowed []string
		want    bool
		method  string
	}{
		{"public", []Spec{{Name: "s", Endpoints: map[string]Endpoint{"Read": {Commands: []Command{{Pattern: "read"}}}}}}, []string{"s"}, true, "Read"},
		{"outside scope", []Spec{{Name: "s", Endpoints: map[string]Endpoint{"Read": {Commands: []Command{{Pattern: "read"}}}}}}, []string{"other"}, false, ""},
		{"private service", []Spec{{Name: "s", Scoped: true, Endpoints: map[string]Endpoint{"Read": {Commands: []Command{{Pattern: "read"}}}}}}, []string{"s"}, false, ""},
		{"write", []Spec{{Name: "s", Endpoints: map[string]Endpoint{"Read": {Writes: true, Commands: []Command{{Pattern: "read"}}}}}}, []string{"s"}, false, ""},
		{"destructive", []Spec{{Name: "s", Endpoints: map[string]Endpoint{"Read": {Destructive: true, Commands: []Command{{Pattern: "read"}}}}}}, []string{"s"}, false, ""},
		{"caller", []Spec{{Name: "s", Endpoints: map[string]Endpoint{"Read": {Needs: Caller, Commands: []Command{{Pattern: "read"}}}}}}, []string{"s"}, false, ""},
		{"account", []Spec{{Name: "s", Endpoints: map[string]Endpoint{"Read": {Needs: Account, Commands: []Command{{Pattern: "read"}}}}}}, []string{"s"}, false, ""},
		{"operator", []Spec{{Name: "s", Endpoints: map[string]Endpoint{"Read": {Needs: Operator, Commands: []Command{{Pattern: "read"}}}}}}, []string{"s"}, false, ""},
		{"two methods", []Spec{{Name: "s", Endpoints: map[string]Endpoint{"A": {Commands: []Command{{Pattern: "read"}}}, "B": {Commands: []Command{{Pattern: "read"}}}}}}, []string{"s"}, false, ""},
		{"two services", []Spec{{Name: "a", Endpoints: map[string]Endpoint{"Read": {Commands: []Command{{Pattern: "read"}}}}}, {Name: "b", Endpoints: map[string]Endpoint{"Read": {Commands: []Command{{Pattern: "read"}}}}}}, []string{"a", "b"}, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			specMu.Lock()
			old := specs
			specs = map[string]Spec{}
			for _, s := range tc.specs {
				specs[s.Name] = s
			}
			specMu.Unlock()
			defer func() { specMu.Lock(); specs = old; specMu.Unlock() }()
			for i := 0; i < 20; i++ {
				got, ok := MatchCommand("read", tc.allowed)
				if ok != tc.want || ok && got.Method != tc.method {
					t.Fatalf("%+v %v", got, ok)
				}
			}
		})
	}
}

func FuzzCommandParsing(f *testing.F) {
	for _, seed := range []string{"headlines", "weather in London", "web search Sam Altman", "weather in ???", "\x00", "東京"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		for _, pattern := range []string{"headlines", "weather in {place}", "web search {query}"} {
			command := Command{Pattern: pattern, Defaults: map[string]any{"limit": 5}}
			a, ok := command.match(input)
			b, again := command.match(input)
			if ok != again || !reflect.DeepEqual(a, b) {
				t.Fatal("nondeterministic parse")
			}
			if ok {
				for _, key := range []string{"place", "query"} {
					if value, exists := a[key]; exists && strings.TrimSpace(value.(string)) == "" {
						t.Fatal("empty argument")
					}
				}
			}
		}
	})
}
