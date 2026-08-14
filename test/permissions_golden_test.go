package test

// What every tool is allowed to do, written down.
//
// This is a safety net for changing the permission model, not a description of
// it. There were five flags — Scoped, Destructive, Account, AccountOnly and a
// dead OptionalAuth — and three of them were three ways of saying "who may call
// this". Collapsing them is worth doing and is exactly the kind of change that
// quietly opens a door.
//
// So the answer is recorded first, for every tool, and has to come out the same
// afterwards. If a refactor is behaviour-preserving this file does not change.
// If it does change, the diff is the list of doors that moved, and somebody has
// to look at each one and say so out loud.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"mu/internal/api"
	"mu/internal/service"
	"mu/tool"
)

const goldenPath = "permissions.golden"

// permissionLines is one line per tool: what it costs, whether it needs a
// caller, whether paying can stand in, and whether the model may hold it.
func permissionLines(t *testing.T) []string {
	t.Helper()
	registerAll(t)
	// Registering a service is not the same as deriving its tools, and the
	// first version of this recorded an empty file and passed — a golden that
	// guards nothing, which is worse than no golden at all because it looks
	// like cover.
	tool.Load(service.Specs())

	// AccountOnly and the registration kind are recorded as well as the derived
	// policy, and that is the point rather than detail. The first version of
	// this logged only the policy, and a scoped service already has
	// needsAccount=true from being scoped — so downgrading mail.Send from
	// "needs an account" to "a wallet will do" changed nothing it could see. It
	// is the distinction being collapsed; if the golden cannot see it, the
	// golden is not guarding the change.
	kind := map[string]string{}
	for _, t := range api.Tools() {
		k := "open"
		if t.HandleCall != nil || t.HandleAuth != nil {
			k = "bound"
		}
		kind[t.Name] = fmt.Sprintf("%s accountOnly=%-5v optionalAuth=%v", k, t.AccountOnly, t.OptionalAuth)
	}

	// Permissions only. Price is deliberately not here: other tests in this
	// package move prices around to check the cost tables, so recording one
	// made this fail depending on which test ran first — and a flaky guard on
	// permissions is a guard somebody switches off. What something costs is
	// quota.json's business and has its own tests. Payable goes with it, being
	// price > 0 and nothing else new.
	var out []string
	for _, p := range api.Policies() {
		out = append(out, fmt.Sprintf("%-28s needsAccount=%-5v destructive=%-5v %s",
			p.Tool, p.NeedsAccount, service.DestructiveTool(p.Tool), kind[p.Tool]))
	}
	sort.Strings(out)
	return out
}

func TestPermissionsAreUnchanged(t *testing.T) {
	lines := permissionLines(t)
	if len(lines) < 80 {
		t.Fatalf("only %d tools have a policy — nothing was derived, so this golden "+
			"would guard an empty file", len(lines))
	}
	got := strings.Join(lines, "\n") + "\n"

	path := filepath.Join(at(""), "test", goldenPath)
	want, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("wrote the first %s — commit it, then this test guards it", goldenPath)
	}
	if err != nil {
		t.Fatal(err)
	}

	if string(want) == got {
		return
	}

	// Say which tools moved, not that a file differs.
	wantLines := strings.Split(strings.TrimRight(string(want), "\n"), "\n")
	gotLines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	before := map[string]string{}
	for _, l := range wantLines {
		before[strings.Fields(l)[0]] = l
	}
	after := map[string]string{}
	for _, l := range gotLines {
		after[strings.Fields(l)[0]] = l
	}
	for name, b := range before {
		a, still := after[name]
		switch {
		case !still:
			t.Errorf("%s is gone", name)
		case a != b:
			t.Errorf("%s changed:\n  was %s\n  now %s", name, b, a)
		}
	}
	for name := range after {
		if _, had := before[name]; !had {
			t.Errorf("%s is new: %s", name, after[name])
		}
	}
	t.Errorf("permissions moved. If every line above is intended, delete %s and "+
		"re-run to record the new answer — deliberately, one look per door", goldenPath)
}
