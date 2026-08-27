package test

// Nobody reads somebody else's data.
//
// A scoped service is one holding a caller's own things — their mail, their
// notes, their files, their wallet. The framework refuses an unauthenticated
// caller at the door, and that is the first half. The second half is that each
// method has to ask *who* is calling and scope its answer to them, and nothing
// checked that. A method that forgot would compile, pass its own tests, and
// return everybody's records to whoever asked first.
//
// So this reads the source and requires every endpoint of every scoped service
// to resolve its caller. It is a source scan rather than a runtime check
// because the failure it is looking for is an absence, and absences do not show
// up in a passing request.
//
// The allowlist below is the interesting part. Three methods legitimately do
// not resolve a caller, each for a different reason, and each reason had to be
// read to be believed. Adding to it should feel like a decision.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// resolvesCaller matches the ways a method learns who is asking.
//
// service.AccountFrom is the real one; the rest are the per-package helpers
// that wrap it. A new helper name means a new entry here, which is a small
// price for a check that reads bodies rather than trusting a naming convention.
//
// Every alternative is a *call*. The first version of this also accepted the
// variable name `owner, err`, and that made it useless: a method rewritten to
// `owner, err := "", error(nil)` — no caller resolved at all — still matched,
// and the deliberately broken build used to test this check passed it. A name
// proves nothing; only the call does.
var resolvesCaller = regexp.MustCompile(
	`AccountFrom|caller\(ctx\)|callerID\(|sender\(ctx|RequireAccount\(`)

// exemptFromScoping is every endpoint on a scoped service that does not resolve
// a caller, with the reason it does not need to.
//
// Each of these was read. None is "it looked fine".
var exemptFromScoping = map[string]string{
	// Passes an empty caller on purpose. images.Search("") selects scope
	// "public", which is the stock pool — the shared images, not everyone's.
	// Handing it the real caller would widen the search to their own private
	// images, which is a different tool.
	"images.Search": "empty caller selects the public pool by design",

	// Resolves the caller through sender(ctx, req), which reads
	// service.AccountFrom and never trusts a field in the request. Matched by
	// the pattern above; listed here so that renaming the helper is a
	// deliberate act rather than a silent hole.
	"mail.Send": "resolves through sender(ctx), which reads AccountFrom",
}

var (
	specBlock   = regexp.MustCompile(`(?s)Spec = service\.Spec\{(.*?)\n\}`)
	isScoped    = regexp.MustCompile(`Scoped:\s*true`)
	endpointMap = regexp.MustCompile(`(?s)Endpoints:\s*map\[string\]service\.Endpoint\{(.*)`)
	endpointKey = regexp.MustCompile(`(?m)^\s*"([A-Za-z0-9_]+)":`)
	handlerFunc = regexp.MustCompile(`(?s)func \((?:\w+ )?Server\) (\w+)\([^)]*\) error \{(.*?)\n\}`)
)

func TestEveryScopedEndpointResolvesItsCaller(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join(at("service"), "*"))
	if err != nil {
		t.Fatal(err)
	}

	checked := 0
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		name := filepath.Base(dir)

		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		var src strings.Builder
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			src.Write(b)
		}
		body := src.String()

		spec := specBlock.FindStringSubmatch(body)
		if spec == nil || !isScoped.MatchString(spec[1]) {
			continue
		}
		eps := endpointMap.FindStringSubmatch(spec[1])
		if eps == nil {
			continue
		}

		bodies := map[string]string{}
		for _, m := range handlerFunc.FindAllStringSubmatch(body, -1) {
			bodies[m[1]] = m[2]
		}

		for _, k := range endpointKey.FindAllStringSubmatch(eps[1], -1) {
			method := k[1]
			checked++

			impl, ok := bodies[method]
			if !ok {
				// Declared in the Spec with no method to match it. A different
				// test catches that; not this one's job to duplicate.
				continue
			}
			if resolvesCaller.MatchString(impl) {
				continue
			}
			if why, exempt := exemptFromScoping[name+"."+method]; exempt {
				t.Logf("%s.%s is exempt: %s", name, method, why)
				continue
			}
			t.Errorf("%s.%s is on a scoped service and never asks who is calling — "+
				"so it answers with whatever it holds, to whoever asked. Scope it to "+
				"service.AccountFrom(ctx), or add it to exemptFromScoping with the "+
				"reason it is safe", name, method)
		}
	}

	if checked < 30 {
		t.Fatalf("only checked %d scoped endpoints — the scan is broken, not the code", checked)
	}
}

// And the allowlist does not rot: an entry naming something that no longer
// exists is a reason nobody has read in a while.
func TestTheScopingAllowlistIsCurrent(t *testing.T) {
	for entry := range exemptFromScoping {
		svc, method, ok := strings.Cut(entry, ".")
		if !ok {
			t.Errorf("%q is not service.Method", entry)
			continue
		}
		files, _ := filepath.Glob(filepath.Join(at("service"), svc, "*.go"))
		found := false
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			if strings.Contains(string(b), ") "+method+"(") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("exemptFromScoping names %s, which no longer exists — delete the "+
				"entry rather than leaving a permission for nothing", entry)
		}
	}
}
