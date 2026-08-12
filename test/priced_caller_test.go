package test

// A price the gateway has nobody to charge is not a price.
//
// The gateway bills the account on the call's context. A tool that dispatches
// with a hard-coded empty account therefore reaches the gateway with nobody to
// bill, and the gateway — correctly, because a priced call with no account has
// already been let in by whichever door admitted it — lets it through free.
//
// web_search was in exactly that state. It declares Cost: web_search, and web
// is not a scoped service, so the derived tool was registered as an open one
// with the caller hard-coded to "". Every search through the tool door went out
// unbilled while /search charged for the same query, which is what the live
// instance showed when two web_search calls moved a balance from 75 to 75.
//
// Landing the gateway did not fix it and could not have: the gateway was never
// the part that was missing. What was missing was the identity, three layers
// up, at the point where a tool is derived from an endpoint.

import (
	"testing"

	"mu/internal/api"
	"mu/tool"
)

// TestEveryPricedToolKnowsWhoIsCalling.
func TestEveryPricedToolKnowsWhoIsCalling(t *testing.T) {
	registerAll(t)
	tool.DeriveTools()

	byName := map[string]api.Tool{}
	for _, tl := range api.Tools() {
		byName[tl.Name] = tl
	}

	for _, spec := range allSpecs() {
		for method, ep := range spec.Endpoints {
			if ep.Cost == "" {
				continue
			}
			name := spec.Name + "_" + toolCase(method)
			tl, ok := byName[name]
			if !ok {
				// Aliased or named some other way; the alias test covers that.
				continue
			}
			if tl.HandleCall == nil && tl.HandleAuth == nil {
				t.Errorf("%s costs %s and is dispatched without a caller, so the gateway "+
					"has nobody to bill and every call through the tool door is free",
					name, ep.Cost)
			}
			if tl.HandleOpen != nil || tl.Handle != nil {
				t.Errorf("%s costs %s and is registered as an open tool", name, ep.Cost)
			}
		}
	}
}

// toolCase lowercases a method name the way a tool name derives from it.
func toolCase(method string) string {
	out := make([]byte, 0, len(method))
	for i := 0; i < len(method); i++ {
		c := method[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out = append(out, c)
	}
	return string(out)
}
