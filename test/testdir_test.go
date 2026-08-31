package test

// `go test ./...` must not be able to touch the instance you actually use.
//
// It could, and it did. Ten packages read the store from func init() — auth
// (accounts, sessions, tokens), user (profiles), app (usage, the API log),
// notes (memory), mail (the relay log, the sent-id filter), and the credential
// stores beside them. Package init runs before TestMain, which runs before the
// first t.Setenv("HOME", t.TempDir()), so by the time any test redirects HOME
// the real ~/.mu has already been read — and internal/app/apilog.go has
// started a goroutine that writes back to it every few seconds for the rest of
// the run.
//
// The evidence: 91 accounts in a developer's live accounts.json, nearly all of
// them fixtures from this suite — act-agentful, chatprivate, csnoop, cthem,
// compose-keeps — beside a scatter of .push_subscriptions.json.tmp and
// .usage.json.tmp files.
//
// internal/dir is the fix. Its own tests hold the behaviour; this holds the
// thing that would quietly undo it — a package resolving the path for itself.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Nothing resolves the home directory for itself any more.
//
// The copies are the bug: one package spelling out $HOME/.mu is one package
// the redirect does not cover, and it fails silently into somebody's live
// data. internal/cli is out — it is a person at a terminal, with no init reads
// and nothing in this suite driving it.
func TestNothingSpellsOutTheHomeDirectory(t *testing.T) {
	home := regexp.MustCompile(`\$HOME/\.mu|UserHomeDir\(\)`)

	for _, file := range goFiles(t) {
		rel := strings.TrimPrefix(filepath.ToSlash(file), "../")
		switch {
		case rel == "internal/dir/root.go": // where it is decided
			continue
		case strings.HasPrefix(rel, "internal/cli/"):
			continue
		case rel == "internal/env/env.go":
			// Reads ~/.env and ~/.mu/.env at init, before internal/dir could
			// help it, and it is the thing that decides what the process is
			// configured with rather than where the store is. Left alone
			// deliberately; it writes nothing.
			continue
		case rel == "service/mail/pgp.go":
			// ~/.gnupg, which is GPG's directory and not ours.
			continue
		}
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if m := home.FindString(string(b)); m != "" {
			t.Errorf("%s resolves the home directory itself (%s) — use dir.Root() "+
				"or dir.Data(), or the suite writes into the live instance through it",
				rel, m)
		}
	}
}
