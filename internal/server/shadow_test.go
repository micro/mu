package server

// What a hand-written tool registration is still worth.
//
// Most tools derive from a service's Spec. The ones in tools.go are written out
// by hand and win wherever the names collide, on the argument that a
// description written for a model beats one produced by reflection. That was
// true when it was written. It stops being true one endpoint at a time, as
// Specs grow Doc strings and request structs grow description tags, and nothing
// notices — a hand-written tool that has become a copy of its derived twin
// costs forty lines and a second place to forget.
//
// This holds the two side by side. It fails on a hand-written tool that has
// become an exact copy, because at that point it is only a copy; a genuine
// difference is not a failure, it is the report, which is the input to deciding
// what to delete.

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"mu/internal/api"
	"mu/internal/service"
)

type shadow struct{ hand, derived api.Tool }

// shadows registers the hand-written half only, then asks what derivation would
// have produced for the same names. Deriving is not run, so nothing here
// depends on which pass wins.
func shadows(t *testing.T) []shadow {
	t.Helper()
	boot()
	registerTools()

	derived := map[string]api.Tool{}
	for _, d := range api.PreviewDerived() {
		derived[d.Name] = d
	}
	if len(derived) < 20 {
		t.Fatalf("only %d endpoints derived a tool — the services did not load", len(derived))
	}

	var out []shadow
	for _, h := range api.Tools() {
		if d, ok := derived[h.Name]; ok {
			out = append(out, shadow{hand: h, derived: d})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].hand.Name < out[j].hand.Name })
	return out
}

func TestNoHandWrittenToolIsAnExactCopy(t *testing.T) {
	list := shadows(t)
	if len(list) == 0 {
		t.Skip("no hand-written tool shadows a derived one")
	}
	for _, s := range list {
		if diffTool(s.hand, s.derived) == "" {
			t.Errorf("%s is written out by hand and is identical to what its Spec "+
				"already derives — delete the registration", s.hand.Name)
		}
	}
}

// The report. `go test ./internal/server -run WhatEach -v` to read it.
func TestWhatEachHandWrittenToolStillAdds(t *testing.T) {
	list := shadows(t)
	t.Logf("%d hand-written tools shadow a derived one", len(list))
	for _, s := range list {
		t.Logf("\n── %s ──\n%s", s.hand.Name, diffTool(s.hand, s.derived))
	}
}

// diffTool reports what the two disagree about, field by field. Empty means
// they are the same tool.
func diffTool(hand, derived api.Tool) string {
	var b strings.Builder
	if norm(hand.Description) != norm(derived.Description) {
		b.WriteString(fmt.Sprintf("  description\n    hand:    %s\n    derived: %s\n",
			short(hand.Description), short(derived.Description)))
	}
	if hand.WalletOp != derived.WalletOp {
		b.WriteString(fmt.Sprintf("  cost: hand %q, derived %q\n", hand.WalletOp, derived.WalletOp))
	}
	if strings.Join(hand.Aliases, ",") != strings.Join(derived.Aliases, ",") {
		b.WriteString(fmt.Sprintf("  aliases: hand %v, derived %v\n", hand.Aliases, derived.Aliases))
	}
	// The field this diff was written without, which is how web_fetch — free,
	// and account-only because fetching any URL a stranger names is a request
	// this server makes on their behalf — was deleted and silently opened to
	// anonymous callers. A comparison is only as good as the fields it knows
	// about, so every field that changes what a caller may do belongs here.
	if hand.AccountOnly != derived.AccountOnly {
		b.WriteString(fmt.Sprintf("  account-only: hand %v, derived %v\n", hand.AccountOnly, derived.AccountOnly))
	}
	if hand.OptionalAuth != derived.OptionalAuth {
		b.WriteString(fmt.Sprintf("  optional-auth: hand %v, derived %v\n", hand.OptionalAuth, derived.OptionalAuth))
	}
	if hand.Method+hand.Path != derived.Method+derived.Path {
		b.WriteString(fmt.Sprintf("  rest: hand %q %q, derived %q %q\n",
			hand.Method, hand.Path, derived.Method, derived.Path))
	}
	if d := diffParams(hand.Params, derived.Params); d != "" {
		b.WriteString("  params\n" + d)
	}
	return b.String()
}

func diffParams(hand, derived []api.ToolParam) string {
	byName := map[string]api.ToolParam{}
	for _, p := range derived {
		byName[p.Name] = p
	}
	var b strings.Builder
	for _, h := range hand {
		d, ok := byName[h.Name]
		if !ok {
			b.WriteString(fmt.Sprintf("    %s: hand only\n", h.Name))
			continue
		}
		delete(byName, h.Name)
		if norm(h.Description) != norm(d.Description) {
			b.WriteString(fmt.Sprintf("    %s: hand %q / derived %q\n",
				h.Name, short(h.Description), short(d.Description)))
		}
		if h.Required != d.Required {
			b.WriteString(fmt.Sprintf("    %s: required hand %v, derived %v\n", h.Name, h.Required, d.Required))
		}
		if h.Type != d.Type {
			b.WriteString(fmt.Sprintf("    %s: type hand %q, derived %q\n", h.Name, h.Type, d.Type))
		}
	}
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		b.WriteString(fmt.Sprintf("    %s: derived only\n", n))
	}
	return b.String()
}

func norm(s string) string { return strings.Join(strings.Fields(s), " ") }

func short(s string) string {
	s = norm(s)
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}

// Not vacuous: both halves have to be present for any of the above to mean
// anything.
func TestTheHarnessSeesBothHalves(t *testing.T) {
	boot()
	registerTools()
	if n := len(service.Specs()); n < 15 {
		t.Fatalf("%d specs registered — the services did not load", n)
	}
	if n := len(api.Tools()); n < 30 {
		t.Fatalf("%d tools registered by hand — tools.go did not run", n)
	}
}
