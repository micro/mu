package test

// One name for a thing, decided in one place.
//
// Two of these drifted at once and neither failed anything:
//
//   - The instance's own agent had two constants — app.SystemUserID and
//     auth.MicroID, both "micro" — so the digest published as one and the
//     opinion agent as the other, through two different doors, and only one of
//     them was metered.
//   - The archive advertised "news, video, market, blog, prayer" in its tool
//     schema. The blog writes "post" and the prayer service writes "reminder",
//     so archive_list kind:"blog" answered "Nothing of kind \"blog\" is
//     archived" on an instance with thousands of posts. A model reading the
//     schema got it wrong every time, because a schema is all it can read.
//
// Both are the same failure: a name copied instead of imported. These tests
// hold the copies down.

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"mu/internal/data"
	"mu/service/archive"
)

// Every kind written is a kind data names.
//
// data.Index's second argument is the word. A literal there is a word invented
// at the call site, which is how a reader comes to advertise a different one.
func TestNoServiceInventsItsOwnKind(t *testing.T) {
	// data.Index(id, KIND, ... — the second argument, on the same line or the
	// next one, which is how the calls in this repo are actually written.
	call := regexp.MustCompile(`data\.(Index|IndexOwned)\(\s*(?:[^,]+),\s*("[a-z]+")`)
	byType := regexp.MustCompile(`data\.(ByType|WithType)\(\s*("[a-z]+")`)

	for _, file := range goFiles(t) {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		rel := strings.TrimPrefix(filepath.ToSlash(file), "../")
		for _, m := range call.FindAllStringSubmatch(string(b), -1) {
			t.Errorf("%s writes kind %s as a literal — use the data.Kind* constant, "+
				"or add one to data.Vocabulary if this is a new kind", rel, m[2])
		}
		for _, m := range byType.FindAllStringSubmatch(string(b), -1) {
			t.Errorf("%s reads kind %s as a literal — a reader that spells the word "+
				"itself is a reader that can spell it wrong", rel, m[2])
		}
	}
}

// And the schema advertises the kinds that exist.
//
// The struct tag has to be a literal — it is read by reflection at registration
// time and cannot be built from a function — so this is what makes it agree
// with data.Vocabulary. Any kind named in a description must be a real one, and
// every real one must be named.
func TestTheAdvertisedKindsAreTheRealOnes(t *testing.T) {
	real := map[string]bool{}
	for _, k := range data.Vocabulary() {
		real[k] = true
	}

	for _, req := range []any{archive.SearchRequest{}, archive.ListRequest{}} {
		typ := reflect.TypeOf(req)
		field, ok := typ.FieldByName("Kind")
		if !ok {
			t.Fatalf("%s has no Kind field", typ.Name())
		}
		desc := field.Tag.Get("description")
		if desc == "" {
			t.Fatalf("%s.Kind has no description, so a model is told nothing", typ.Name())
		}

		named := kindsIn(desc, real)
		for _, k := range named {
			if !real[k] {
				t.Errorf("%s.Kind advertises %q, which nothing writes — a call with it "+
					"returns nothing and reads as an empty archive", typ.Name(), k)
			}
		}
		for k := range real {
			if !strings.Contains(desc, k) {
				t.Errorf("%s.Kind does not mention %q, so a whole kind of the archive is "+
					"unreachable by anything reading the schema", typ.Name(), k)
			}
		}
	}
}

// kindsIn pulls the comma-separated list of words out of a description.
//
// The words are in a sentence, so this takes the lowercase single words and
// ignores the prose around them: it is looking for a kind that is named and
// wrong, and an English word that happens to be a real kind is not a false
// positive worth avoiding.
func kindsIn(desc string, real map[string]bool) []string {
	var out []string
	for _, w := range regexp.MustCompile(`[^a-z]+`).Split(strings.ToLower(desc), -1) {
		switch w {
		case "", "narrow", "to", "one", "kind", "omit", "search", "everything", "for",
			"a", "summary", "of", "what", "is", "here":
			continue
		}
		out = append(out, w)
	}
	return out
}

// The instance's own agent is named once.
func TestTheAgentHasOneName(t *testing.T) {
	for _, file := range goFiles(t) {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		rel := strings.TrimPrefix(filepath.ToSlash(file), "../")
		if rel == "internal/auth/micro.go" {
			continue // where it is declared
		}
		src := string(b)
		if strings.Contains(src, "SystemUserID") || strings.Contains(src, "SystemUserName") {
			t.Errorf("%s names the agent as SystemUser* — it has an account, and its "+
				"name is auth.MicroID / auth.MicroName", rel)
		}
		// A bare "micro" as an id is the same copy by another route.
		if regexp.MustCompile(`(AuthorID|accountID|caller|account)\s*[:=]=?\s*"micro"`).MatchString(src) {
			t.Errorf(`%s writes the agent's id as the literal "micro" — use auth.MicroID`, rel)
		}
	}
}

// goFiles is the product's non-test Go source.
func goFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
