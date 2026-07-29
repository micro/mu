package api

import (
	"context"
	"testing"
)

// A renamed tool must stay callable by its old name — the compatibility promise
// depends on it — while the old name stays out of the catalogue.
func TestMCPAliasesResolveButAreNotListed(t *testing.T) {
	saved := tools
	defer func() { tools = saved }()

	tools = []Tool{{
		Name:        "thing_list",
		Aliases:     []string{"thing"},
		Description: "d",
		Handle:      func(map[string]any) (string, error) { return "ok", nil },
	}}

	res := mcpResolver()

	listed, err := res.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, tl := range listed {
		if tl.Name == "thing" {
			t.Error("retired name should not appear in the catalogue")
		}
	}

	for _, name := range []string{"thing_list", "thing"} {
		out, err := res.Call(context.Background(), name, map[string]any{})
		if err != nil {
			t.Errorf("Call(%q) failed: %v", name, err)
			continue
		}
		if out == nil || out.Text != "ok" {
			t.Errorf("Call(%q) returned %+v", name, out)
		}
	}

	if _, err := res.Call(context.Background(), "nope", map[string]any{}); err == nil {
		t.Error("unknown tool should still error")
	}
}
