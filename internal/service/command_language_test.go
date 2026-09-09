package service

import (
	"reflect"
	"testing"
)

func TestCommandLanguageComposesOnlyCompleteReads(t *testing.T) {
	specMu.Lock()
	old := specs
	specs = map[string]Spec{
		"news":    {Name: "news", Handler: ListProbe{}, Endpoints: map[string]Endpoint{"List": {Commands: []Command{{Pattern: "headlines"}}}}},
		"weather": {Name: "weather", Endpoints: map[string]Endpoint{"Lookup": {Commands: []Command{{Pattern: "weather"}, {Pattern: "weather in {place}"}, {Pattern: "weather in {place}", Suffix: "tomorrow", Defaults: map[string]any{"day": "tomorrow"}}}}}},
		"web":     {Name: "web", Endpoints: map[string]Endpoint{"Search": {Commands: []Command{{Pattern: "web search {query}"}}}}},
		"mail":    {Name: "mail", Scoped: true, Handler: ListProbe{}, Endpoints: map[string]Endpoint{"List": {Needs: Caller}, "Send": {Writes: true, Commands: []Command{{Pattern: "send mail"}}}}},
	}
	specMu.Unlock()
	defer func() { specMu.Lock(); specs = old; specMu.Unlock() }()
	allowed := []string{"news", "weather", "web", "mail"}
	for _, tc := range []struct {
		input string
		count int
	}{
		{"news", 1}, {"Please news", 1}, {"can you show me news?", 1}, {"could you weather in London", 1},
		{"weather and latest news", 2}, {"weather & news", 2}, {"PLEASE weather in London tomorrow and headlines!", 2},
		{"weather and news and weather and news", 4},
		{"web search Sam Altman and OpenAI", 1}, {`web search "news and weather"`, 1},
		{"don't show news", 0}, {"please don't show news", 0}, {"news and send mail", 0}, {"weather and compare my plans", 0},
		{"news then weather", 0}, {"weather in London and Paris", 0}, {"news and", 0}, {"and news", 0},
		{"news and news and news and news and news", 0}, {"news\nweather", 0}, {"news and mail", 0},
		{"weather in London tomorrow and email it to me", 0}, {"weather in London yesterday", 0},
	} {
		calls, ok := MatchCommandsFor(tc.input, allowed, false)
		if ok != (tc.count > 0) || len(calls) != tc.count {
			t.Errorf("%q: %+v %v", tc.input, calls, ok)
		}
	}
	calls, ok := MatchCommandsFor("please weather in São Paulo tomorrow and news", allowed, false)
	if !ok || !reflect.DeepEqual(calls[0].Args, map[string]any{"place": "São Paulo", "day": "tomorrow"}) {
		t.Fatalf("lost slots: %+v", calls)
	}
	calls, ok = MatchCommandsFor("web search Sam Altman and OpenAI", allowed, false)
	if !ok || calls[0].Args["query"] != "Sam Altman and OpenAI" {
		t.Fatalf("changed query: %+v", calls)
	}
	if _, ok := MatchCommandsFor("news and mail", allowed, true); !ok {
		t.Fatal("owner's private read was lost")
	}
	if _, ok := MatchCommandsFor("news and weather", []string{"news"}, true); ok {
		t.Fatal("widened scope")
	}
}
