package service

import (
	"context"
	"os"
	"strings"
	"testing"

	"go-micro.dev/v6/agent"
)

type TempProbe struct{}

type ProbeReq struct {
	City string `json:"city" description:"City name"`
}
type ProbeRsp struct {
	Report string `json:"report" description:"Temperature report"`
}

// Temp returns the temperature for a city.
// @example {"city": "London"}
func (TempProbe) Temp(_ context.Context, req *ProbeReq, rsp *ProbeRsp) error {
	rsp.Report = "The temperature in " + req.City + " is 16 degrees and it is overcast."
	return nil
}

// TestNewAgentLive proves a go-micro agent wired to the mesh discovers a
// registered service and calls its method to answer. Gated on ATLAS_API_KEY.
func TestNewAgentLive(t *testing.T) {
	key := os.Getenv("ATLAS_API_KEY")
	if key == "" {
		t.Skip("set ATLAS_API_KEY to run")
	}
	if err := Register("probe", TempProbe{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	a := NewAgent("assistant",
		"You are a helpful assistant. Use the tools, then answer in plain language.",
		"atlascloud", key, []string{"probe"},
		agent.Model("deepseek-ai/deepseek-v4-pro"))
	go a.Run()
	defer a.Stop()

	resp, err := a.Ask(context.Background(), "What's the temperature in London right now?")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	t.Logf("reply: %s", resp.Reply)
	if !strings.Contains(resp.Reply, "16") {
		t.Fatalf("agent did not use the service tool result: %q", resp.Reply)
	}
}
