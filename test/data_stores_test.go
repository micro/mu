package test

// One file, one owner.
//
// wallets.json was written by two different maps — the credit ledger in
// account/credits.go and the key store in service/wallet/basewallet.go — because a rename
// moved the second onto the first's filename. Each save destroyed the other's
// contents. It cost an account its balance and very nearly cost a wallet the
// private key to real money; the key survived only because a legacy file is
// never written.
//
// Nothing caught it. Both packages compiled, both tests passed, and the two
// writers never appear in the same file, so reading either one tells you
// nothing. This is the check that does.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// saveCall matches a write to a store: data.SaveJSON(<file>, <value>).
//
// The value matters as much as the file. Two writes to one filename are fine
// when they save the same map — one store, split across files. Two writes
// saving *different* maps is the bug: whichever runs last replaces the other's
// contents wholesale.
var saveCall = regexp.MustCompile(`data\.Save(?:JSON|File)\(\s*([A-Za-z0-9_."/-]+)\s*,\s*([A-Za-z0-9_.\[\]]+)`)

// fileVar matches a package variable holding a store filename, so a write
// through `walletsFile` resolves to the file it names. The collision this test
// exists for went through exactly such a variable.
var fileVar = regexp.MustCompile(`([A-Za-z0-9_]+)\s*=\s*"([a-z0-9_-]+\.json)"`)

// notAStore is repository data compiled into the binary, not per-instance state.
var notAStore = map[string]bool{"quota.json": true, "server.json": true}

func TestNoTwoStoresShareAFile(t *testing.T) {
	// store file -> what gets saved into it -> where from
	saved := map[string]map[string][]string{}

	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if strings.Contains(filepath.ToSlash(path), "internal/data/") {
			return nil // the store itself names files on everybody's behalf
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		src := string(b)

		vars := map[string]string{}
		for _, m := range fileVar.FindAllStringSubmatch(src, -1) {
			vars[m[1]] = m[2]
		}

		for _, m := range saveCall.FindAllStringSubmatch(src, -1) {
			file := strings.Trim(m[1], `"`)
			if resolved, ok := vars[m[1]]; ok {
				file = resolved
			}
			if !strings.HasSuffix(file, ".json") || notAStore[file] {
				continue
			}
			if saved[file] == nil {
				saved[file] = map[string][]string{}
			}
			saved[file][m[2]] = append(saved[file][m[2]], filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) == 0 {
		t.Fatal("found no store writes at all — the pattern has stopped matching")
	}

	for file, values := range saved {
		if len(values) < 2 {
			continue
		}
		var lines []string
		for value, where := range values {
			sort.Strings(where)
			lines = append(lines, "  "+value+" from "+strings.Join(where, ", "))
		}
		sort.Strings(lines)
		t.Errorf("%s is written from %d different values — each save replaces the other:\n%s",
			file, len(values), strings.Join(lines, "\n"))
	}
}

// TestTheKeyStoreAndTheLedgerAreSeparate is the specific case, named so a
// future rename cannot quietly recreate it.
func TestTheKeyStoreAndTheLedgerAreSeparate(t *testing.T) {
	read := func(p string) string {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	ledger := read("../account/credits.go")
	keys := read("../service/wallet/basewallet.go")

	if !strings.Contains(ledger, `"wallets.json"`) {
		t.Error("the credit ledger no longer names wallets.json; update this test deliberately")
	}
	if strings.Contains(keys, `walletsFile = "wallets.json"`) {
		t.Fatal("the key store is back on the credit ledger's file — this wipes balances and keys")
	}
	if !strings.Contains(keys, `walletsFile = "base_wallets.json"`) {
		t.Error("the key store's filename changed; make sure it is not one another store owns")
	}
}
