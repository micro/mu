package test

// Every service is actually stood up.
//
// SMS shipped with a page, a route, four tools, a price and rows in two
// documents, and was in none of them on the live site: the one line that
// registers it never made it into boot.go. Nothing caught that. It compiles
// either way — an unregistered service is not a broken reference, it is an
// absence — and the policy tests build their own list of Specs rather than
// booting, so they were checking a catalogue nobody serves.
//
// A Spec that nothing loads is a service that does not exist, however finished
// it looks in its own directory.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestEveryServiceIsLoadedAtBoot pairs each package holding a Spec with the
// call that registers it.
func TestEveryServiceIsLoadedAtBoot(t *testing.T) {
	boot := serverFiles(t)

	pkgs := packagesWithSpecs(t)
	if len(pkgs) < 15 {
		t.Fatalf("only found %d services — the scan is broken, not the code", len(pkgs))
	}

	for _, pkg := range pkgs {
		// Either name: some register with Load, some with LoadService, and
		// which one is not the point being made here.
		loaded := regexp.MustCompile(`\b` + regexp.QuoteMeta(pkg) + `\.Load(Service)?\(\)`)
		if !loaded.Match(boot) {
			t.Errorf("service/%s declares a Spec and nothing calls %s.Load() — "+
				"it has a page, tools and a price, and is in no catalogue", pkg, pkg)
		}
	}
}

// TestNoServiceIsImportedUnderAnAlias keeps the check above honest.
//
// service/notes was imported as memsvc, three renames after anything was called
// memory. That is how the missing registration hid: an edit that looked for
// notes.LoadService() found nothing to sit beside, changed nothing, and said
// nothing — and an alias is exactly what makes a package hard to find by the
// name it actually has.
func TestNoServiceIsImportedUnderAnAlias(t *testing.T) {
	alias := regexp.MustCompile(`(?m)^\s*([a-z][a-zA-Z0-9_]*)\s+"mu/service/([a-z/]+)"`)
	for _, file := range []string{
		filepath.Join("internal", "server", "boot.go"),
		filepath.Join("internal", "server", "routes.go"),
		filepath.Join("internal", "server", "hooks.go"),
	} {
		b, err := os.ReadFile(at(file))
		if err != nil {
			continue
		}
		for _, m := range alias.FindAllStringSubmatch(string(b), -1) {
			name, path := m[1], m[2]
			// An alias matching the package's own last element is the compiler
			// being told what it already knows, which is harmless.
			leaf := path
			if parts := strings.Split(path, "/"); len(parts) > 0 {
				leaf = parts[len(parts)-1]
			}
			if name == leaf {
				continue
			}
			// A medium with both halves has two packages of one name — a
			// service for what an agent can send and a client for how a person
			// arrives — and one of them has to be aliased. The client was there
			// first and its call sites are not the new work's to churn, so the
			// newcomer carries it.
			if _, err := os.Stat(at("client", leaf)); err == nil {
				continue
			}
			t.Errorf("%s imports mu/service/%s as %q — call a service by its name, "+
				"so that looking for it finds it", file, path, name)
		}
	}
}

// serverFiles is every file that stands services up, read as one.
func serverFiles(t *testing.T) []byte {
	t.Helper()
	var all []byte
	for _, name := range []string{"boot.go", "hooks.go", "routes.go", "server.go"} {
		b, err := os.ReadFile(at(filepath.Join("internal", "server", name)))
		if err != nil {
			continue
		}
		all = append(all, b...)
	}
	if len(all) == 0 {
		t.Fatal("could not read internal/server")
	}
	return all
}

// packagesWithSpecs returns the package name of every service under service/.
func packagesWithSpecs(t *testing.T) []string {
	t.Helper()
	spec := regexp.MustCompile(`Spec = service\.Spec\{`)
	pkgName := regexp.MustCompile(`(?m)^package ([a-z][a-zA-Z0-9_]*)`)

	seen := map[string]bool{}
	var out []string
	err := filepath.Walk(at("service"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil || !spec.Match(b) {
			return nil
		}
		m := pkgName.FindSubmatch(b)
		if m == nil {
			return nil
		}
		if name := string(m[1]); !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestEveryServiceIsInThePolicyList closes the same gap one level up.
//
// allSpecs in spec_policy_test.go is hand-written, and it is what the policy and
// documentation tests see. A service missing from it is not merely undocumented:
// it is invisible to every check that exists to notice it is undocumented. The
// comment on that list already records notes going missing for exactly that
// reason, which is a comment doing a test's job.
func TestEveryServiceIsInThePolicyList(t *testing.T) {
	b, err := os.ReadFile(at("test", "spec_policy_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	list := string(b)

	// The name a Spec is reached by is the import alias where there is one:
	// whatsapp is imported as whatsappsvc, because client/whatsapp already
	// holds the plain name.
	imported := regexp.MustCompile(`(?m)^\s*(?:([a-z][a-zA-Z0-9_]*)\s+)?"mu/service/([a-z/]+)"`)
	name := map[string]string{}
	for _, m := range imported.FindAllStringSubmatch(list, -1) {
		alias, path := m[1], m[2]
		leaf := path
		if parts := strings.Split(path, "/"); len(parts) > 0 {
			leaf = parts[len(parts)-1]
		}
		if alias == "" {
			alias = leaf
		}
		name[leaf] = alias
	}

	for _, pkg := range packagesWithSpecs(t) {
		ident, ok := name[pkg]
		if ok && regexp.MustCompile(`\b`+regexp.QuoteMeta(ident)+`\.Spec\b`).MatchString(list) {
			continue
		}
		t.Errorf("service/%s declares a Spec and allSpecs does not list it — "+
			"the documentation tests cannot see a service they were not handed", pkg)
	}
}

// TestPublicServicesUseTheSharedGate keeps the auth rule from drifting apart
// again.
//
// The rule is that an operation costing this instance money needs somebody to
// bill, and one costing nothing needs nobody. It lived in service/places and
// nowhere else, so weather, news search, web search, web fetch and article
// reading each grew their own gate that demanded a session first and asked
// about credits second. On a self-hosted instance that refuses a guest for a
// call nobody could be charged for.
//
// A handler on a service whose page is public may still require a session — for
// posting, for anything account-scoped. What it must not do is pair
// RequireSession with CheckQuota, which is the shape of deciding metering by
// hand instead of asking app.BillableCaller.
func TestPublicServicesUseTheSharedGate(t *testing.T) {
	// Services answering questions about public data, where a guest on a free
	// instance should get an answer.
	public := []string{"weather", "news", "search", "places", "flights", "markets", "video"}

	for _, name := range public {
		dir := at("service", name)
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil || len(files) == 0 {
			continue
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			b, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			src := string(b)
			if !strings.Contains(src, "quota.CheckQuota") {
				continue
			}
			if strings.Contains(src, "auth.RequireSession") {
				t.Errorf("%s pairs auth.RequireSession with quota.CheckQuota — that is "+
					"the hand-rolled gate. Use app.BillableCaller, which refuses a guest "+
					"only where this instance can actually charge for the call",
					strings.TrimPrefix(file, at("")))
			}
		}
	}
}
