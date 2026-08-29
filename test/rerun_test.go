package test

// A script that soft navigation re-runs must not declare at the top level.
//
// Arriving at a page by clicking a link does not reload the document: #content
// is replaced and every <script> inside it is re-created so the page's
// behaviour comes with it. Those scripts therefore run a second, third and
// fourth time in one document — and `const X` at the top level of the second
// run throws "Identifier 'X' has already been declared" before a single
// statement executes. A SyntaxError kills the whole script, not the one line.
//
// Reported as: recent searches missing on /web when you arrive via Open, and
// there after a refresh. A refresh is a new document, where the declaration is
// the first one — which is why it looked like a loading problem and was a
// scoping one. The same block had been copied into /video.
//
// The fix is either a function scope or var, depending on whether the script
// has to leave anything global for an onclick to find.
//
// # Why this counts braces instead of reading indentation
//
// The first version of this test matched a const at four spaces or fewer and
// reported four more files. Every one was a false positive: three were Go
// `const` declarations, because the <script>…</script> match had run past the
// end of one string literal and into the source around it, and the fourth was
// JavaScript indented two spaces inside a function. Acting on that output
// corrupted four files. So: raw string literals first, then depth.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var scriptOpen = regexp.MustCompile(`<script[^>]*>`)

// topLevelDecl finds `const foo` / `let foo` at the start of a statement.
var declAt = regexp.MustCompile(`(?:^|[;{}\n])\s*(const|let)\s+[A-Za-z_$]`)

func TestInlineScriptsSurviveBeingRunTwice(t *testing.T) {
	root := repoRoot(t)

	var bad []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "vendor", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)

		// One raw string at a time. A <script> in one literal and a </script>
		// in another are not one script, and matching across them reads the Go
		// source between as JavaScript.
		for _, lit := range rawStrings(string(b)) {
			for _, body := range scriptBodies(lit) {
				if name := topLevelDeclaration(body); name != "" {
					bad = append(bad, rel+": "+name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(bad) > 0 {
		t.Errorf("soft navigation re-runs these scripts in the same document, and a "+
			"repeated top-level const/let throws before anything in the block runs. "+
			"Use var, or wrap the block in (function () { … })() if nothing in it has "+
			"to stay global. %d place(s):\n  %s", len(bad), strings.Join(bad, "\n  "))
	}
}

// rawStrings returns the contents of every backtick-delimited Go literal.
func rawStrings(src string) []string {
	var out []string
	for {
		i := strings.IndexByte(src, '`')
		if i < 0 {
			return out
		}
		src = src[i+1:]
		j := strings.IndexByte(src, '`')
		if j < 0 {
			return out
		}
		out = append(out, src[:j])
		src = src[j+1:]
	}
}

// scriptBodies returns what sits between each <script …> and its </script>.
func scriptBodies(lit string) []string {
	var out []string
	rest := lit
	for {
		m := scriptOpen.FindStringIndex(rest)
		if m == nil {
			return out
		}
		rest = rest[m[1]:]
		end := strings.Index(rest, "</script>")
		if end < 0 {
			return out
		}
		out = append(out, rest[:end])
		rest = rest[end:]
	}
}

// topLevelDeclaration names the first const/let declared at brace depth zero,
// or "" if there is none. Comments and strings are blanked first so a brace
// inside either does not move the depth.
func topLevelDeclaration(body string) string {
	clean := blankNonCode(body)
	depth := 0
	for i := 0; i < len(clean); i++ {
		switch clean[i] {
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		}
		if depth != 0 {
			continue
		}
		if m := declAt.FindStringSubmatchIndex(clean[i:]); m != nil && m[0] == 0 {
			return strings.TrimSpace(clean[i : i+m[1]])
		}
	}
	return ""
}

// blankNonCode replaces comments and string bodies with spaces, keeping length
// and newlines so offsets and the brace count stay honest.
func blankNonCode(s string) string {
	out := []byte(s)
	blank := func(from, to int) {
		for k := from; k < to && k < len(out); k++ {
			if out[k] != '\n' {
				out[k] = ' '
			}
		}
	}
	for i := 0; i < len(s); i++ {
		switch {
		case strings.HasPrefix(s[i:], "//"):
			j := strings.IndexByte(s[i:], '\n')
			if j < 0 {
				j = len(s) - i
			}
			blank(i, i+j)
			i += j
		case strings.HasPrefix(s[i:], "/*"):
			j := strings.Index(s[i:], "*/")
			if j < 0 {
				j = len(s) - i
			} else {
				j += 2
			}
			blank(i, i+j)
			i += j
		case s[i] == '\'' || s[i] == '"':
			q := s[i]
			j := i + 1
			for j < len(s) && s[j] != q {
				if s[j] == '\\' {
					j++
				}
				if s[j] == '\n' {
					break
				}
				j++
			}
			blank(i+1, j)
			i = j
		}
	}
	return string(out)
}
