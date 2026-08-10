package test

// A hand-written tool reads the parameters it declares, and declares the ones
// it reads.
//
// For a tool derived from a Spec the two cannot disagree: the request struct is
// the schema, so a field is both at once. A hand-registered tool declares
// Params in one place and reads args["..."] in another, and nothing connects
// them — so a rename in one is silent in the other.
//
// That is not hypothetical. places_search declared "query" and its handler read
// "q", so an agent doing exactly what the schema said got an empty search and
// no error. It was found by calling the live instance, which is a poor way to
// find a mismatch that is visible in the file.
//
// The failure is quiet in both directions. A declared-but-unread parameter is
// advertised and ignored; a read-but-undeclared one works only for a caller who
// guesses it.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	toolBlockRe = regexp.MustCompile(`(?s)api\.Tool\{.*?\n\t\}\)`)
	toolNameRe  = regexp.MustCompile(`Name:\s*"([a-z][a-z0-9_]*)"`)
	paramNameRe = regexp.MustCompile(`\{Name:\s*"([a-z][a-z0-9_]*)",\s*Type:`)
	argsReadRe  = regexp.MustCompile(`args\[\s*"([a-z][a-z0-9_]*)"\s*\]`)
	// The helper closures a handler defines to read args: str("from"),
	// num("to_lat"), b("public"). Declared as `func(k string)` and called with
	// a literal.
	helperRe = regexp.MustCompile(`\b(?:str|num|b|s|f|i|arg|argString|argFloat)\(\s*"([a-z][a-z0-9_]*)"\s*\)`)
)

func TestHandWrittenToolsReadTheParametersTheyDeclare(t *testing.T) {
	src, err := os.ReadFile(at("internal/server/tools.go"))
	if err != nil {
		t.Fatal(err)
	}

	checked := 0
	for _, block := range toolBlockRe.FindAllString(string(src), -1) {
		// Only tools with an inline handler: a path-backed tool passes its args
		// on as a query string and has nothing to compare against here.
		if !strings.Contains(block, "Handle:") && !strings.Contains(block, "HandleAuth:") {
			continue
		}
		nm := toolNameRe.FindStringSubmatch(block)
		if nm == nil {
			continue
		}
		name := nm[1]

		declared := map[string]bool{}
		for _, m := range paramNameRe.FindAllStringSubmatch(block, -1) {
			declared[m[1]] = true
		}
		if len(declared) == 0 {
			continue // a tool taking no arguments has nothing to drift
		}

		read := map[string]bool{}
		for _, re := range []*regexp.Regexp{argsReadRe, helperRe} {
			for _, m := range re.FindAllStringSubmatch(block, -1) {
				read[m[1]] = true
			}
		}
		if len(read) == 0 {
			continue // reads its arguments some way this cannot see
		}
		checked++

		for _, p := range sorted(declared) {
			if !read[p] {
				t.Errorf("%s declares the parameter %q and never reads it — an agent "+
					"passing what the schema asks for is ignored", name, p)
			}
		}
		for _, p := range sorted(read) {
			if !declared[p] {
				t.Errorf("%s reads the argument %q without declaring it — it works "+
					"only for a caller who guesses", name, p)
			}
		}
	}

	// This scan sees one of the ten hand-written tools that are left, and that
	// is worth stating rather than hiding behind a threshold: the other nine
	// read their arguments in ways the patterns below cannot follow, so a
	// pass here is not coverage of the hand-written surface. It is coverage of
	// whichever tools happen to be written in the shape it can read.
	//
	// The population this test has anything to say about is shrinking on
	// purpose. A derived tool cannot drift this way — its schema is generated
	// from the request struct its handler is handed, so declaring a parameter
	// and never reading it is not expressible. Only tools written out by hand,
	// which declare a schema in one place and pull arguments out of a map in
	// another, can disagree with themselves.
	//
	// Thirty-odd of those became derived. The floor is what is left rather than
	// a number chosen to pass: it still catches the patterns silently ceasing
	// to match, and when it reaches zero this test has no subject and should be
	// deleted rather than propped up.
	if checked < 1 {
		t.Errorf("only %d tools were compared; either the patterns have stopped "+
			"matching, or the last hand-written tools with readable argument "+
			"handling are gone and this test should go with them", checked)
	}
	t.Logf("compared declared and read parameters for %d hand-written tools", checked)
}

func sorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
