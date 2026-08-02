package service

import (
	"context"
	"testing"

	"go-micro.dev/v6/ai"
)

// DocsProbe is a handler used to prove the Docs wiring end to end.
type DocsProbe struct{}

type ProbeLookRequest struct {
	Query string `json:"query"`
}
type ProbeLookResponse struct {
	Text string `json:"text"`
}

func (DocsProbe) Look(_ context.Context, _ *ProbeLookRequest, _ *ProbeLookResponse) error {
	return nil
}

// TestDocsReachTheAgentToolList proves a Docs entry survives the whole path:
// Register -> go-micro endpoint metadata -> registry -> the tool list the agent
// is handed. Without it the agent is given "Call Look on docsprobe service".
func TestDocsReachTheAgentToolList(t *testing.T) {
	const want = "Look something up in the probe"
	if err := Register("docsprobe", new(DocsProbe), Docs{"Look": want}); err != nil {
		t.Fatal(err)
	}

	if got := EndpointDescriptions("docsprobe")["DocsProbe.Look"]; got != want {
		t.Fatalf("registry has %q, want %q", got, want)
	}

	tools, err := ai.DiscoverTools(Registry())
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range tools {
		if tl.OriginalName == "docsprobe.DocsProbe.Look" {
			if tl.Description != want {
				t.Fatalf("agent sees %q, want %q", tl.Description, want)
			}
			return
		}
	}
	t.Fatal("probe tool was not discovered at all")
}

// TestDocsAreOptional keeps Register usable without them.
func TestDocsAreOptional(t *testing.T) {
	if err := Register("docsprobe2", new(DocsProbe)); err != nil {
		t.Fatal(err)
	}
	if got := EndpointDescriptions("docsprobe2")["DocsProbe.Look"]; got != "" {
		t.Fatalf("expected no description, got %q", got)
	}
}
