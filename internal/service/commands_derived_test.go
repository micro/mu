package service

import (
	"context"
	"strings"
	"testing"
)

type ListProbe struct{}
type ListProbeRequest struct {
	Limit int `json:"limit"`
}
type ListProbeResponse struct {
	Text string `json:"text"`
}

func (ListProbe) List(context.Context, *ListProbeRequest, *ListProbeResponse) error { return nil }

type RequiredListProbe struct{}
type RequiredListRequest struct {
	ID string `json:"id" required:"true"`
}

func (RequiredListProbe) List(context.Context, *RequiredListRequest, *ListProbeResponse) error {
	return nil
}

func TestDerivedListCommandsAndDiscovery(t *testing.T) {
	specMu.Lock()
	old := specs
	specs = map[string]Spec{
		"video":    {Name: "video", Handler: ListProbe{}, Endpoints: map[string]Endpoint{"List": {Aliases: []string{"videos"}}}},
		"private":  {Name: "private", Scoped: true, Handler: ListProbe{}, Endpoints: map[string]Endpoint{"List": {Needs: Caller}}},
		"required": {Name: "required", Handler: RequiredListProbe{}, Endpoints: map[string]Endpoint{"List": {}}},
		"write":    {Name: "write", Handler: ListProbe{}, Endpoints: map[string]Endpoint{"List": {Writes: true}}},
		"operator": {Name: "operator", Handler: ListProbe{}, Endpoints: map[string]Endpoint{"List": {Needs: Operator}}},
	}
	specMu.Unlock()
	defer func() { specMu.Lock(); specs = old; specMu.Unlock() }()
	allowed := []string{"video", "private", "required", "write", "operator"}
	for _, input := range []string{"video", "Video", "videos", "latest videos", `"latest videos"`, "‘latest videos’", "show me videos", "  LATEST   VIDEOS! "} {
		call, ok := MatchCommandFor(input, allowed, false)
		if !ok || call.Service != "video" || call.Method != "List" || call.Args["limit"] != 5 {
			t.Fatalf("%q: %+v %v", input, call, ok)
		}
	}
	for _, input := range []string{"required", "write", "operator", "don't show video", "video and weather"} {
		if _, ok := MatchCommandFor(input, allowed, true); ok {
			t.Fatalf("unsafe inference: %s", input)
		}
	}
	if _, ok := MatchCommandFor("private", allowed, false); ok {
		t.Fatal("guest can read private")
	}
	if _, ok := MatchCommandFor("private", allowed, true); !ok {
		t.Fatal("owner cannot read own service")
	}
	if _, ok := MatchCommandFor("private", []string{"video"}, true); ok {
		t.Fatal("scope widened")
	}
	for _, private := range []bool{false, true} {
		examples := CommandExamples(allowed, private)
		if strings.Contains(strings.Join(examples, ","), "private") != private {
			t.Fatalf("wrong discovery: %v", examples)
		}
		for _, example := range examples {
			if _, ok := MatchCommandFor(example, allowed, private); !ok {
				t.Fatalf("non-executable shortcut: %s", example)
			}
		}
	}
}
