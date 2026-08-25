package api

import (
	"os"
	"testing"

	"mu/internal/quota"
)

// TestMain points HOME at a temporary directory so tests that create accounts
// and tokens do not write into the real ~/.mu. Same reason as the one in
// internal/auth: a test in this session minted real tokens on a real account.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu_api_test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)

	// Prices reach internal/quota from main, which no test binary has. Without
	// this every tool reads as costing the 1-credit fallback, and the policy
	// tests below — which are about what is priced and what is not — would be
	// asserting over a flat list.
	if err := quota.LoadFromTree(); err != nil {
		panic("api tests need quota.json: " + err.Error())
	}

	// A fixture, because this package no longer ships tools of its own.
	//
	// It used to declare twenty-five, and the tests below asserted over them —
	// which is how a protocol test came to fail whenever the catalogue moved.
	// The catalogue is built in tool/ from the service Specs now and handed
	// here, so what is left to test is the protocol: does tools/list answer
	// with well-formed entries, does a metered call reach the quota gate. Both
	// need a tool; neither needs a particular one.
	RegisterTool(Tool{
		Name:        "probe_free",
		Title:       "Probe",
		Description: "A tool that exists so the protocol has something to answer about",
		Params: []ToolParam{
			{Name: "query", Type: "string", Description: "Anything", Required: true},
		},
		Handle: func(map[string]any) (string, error) { return "ok", nil },
	})
	RegisterTool(Tool{
		Name:        "probe_paid",
		Title:       "Paid probe",
		Description: "A priced tool, so the quota gate has something to refuse",
		WalletOp:    quota.OpWebSearch,
		Params: []ToolParam{
			{Name: "query", Type: "string", Description: "Anything", Required: true},
		},
		Handle: func(map[string]any) (string, error) { return "ok", nil },
	})

	// Snapshot the real tool surface before any test body runs.
	//
	// Several tests register probe services to exercise derivation, and those
	// probes are deliberately terse. Linting the live registry would therefore
	// pass or fail depending on which tests had run first — the same
	// order-dependence that made two other tests in this repo lie today. The
	// lint reads this instead, which is the surface an agent actually sees.
	shipped = mcpTools()

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
