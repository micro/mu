package cli

// One word, one meaning.
//
// `wallet` named two different keys: one the server holds for your account,
// one you hold on this machine and the server never sees. Both are wallets in
// plain English and they are not interchangeable in any way that matters —
// different owners, different risks, different recovery.
//
// The first attempt at fixing it kept both under the word and added a rule
// about which arguments meant which: `mu wallet` was local, `mu wallet balance`
// was the service. That rule *is* the overload. It works and it is unlearnable,
// because nothing about the words tells you which key you are about to look at.
//
// So the local one is `mu x402 key` — under the thing it exists for — and
// `wallet` is an ordinary service reaching the ordinary dispatcher. This checks
// neither special case has grown back.

import (
	"os"
	"strings"
	"testing"
)

func TestWalletHasNoSpecialCaseInTheDispatcher(t *testing.T) {
	src, err := os.ReadFile("dispatch.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	if strings.Contains(body, `case "wallet":`) {
		t.Error(`dispatch.go has a case for "wallet" again. It is a service, and a ` +
			`service reaches the generic dispatcher like every other one — a case ` +
			`here means the CLI is deciding between two things called wallet, which ` +
			`is the thing that was wrong`)
	}
	if strings.Contains(body, `case "key":`) {
		t.Error(`"key" is a top-level command again. It belongs under x402, which is ` +
			`what it is for — and this CLI already deals in keys that have nothing ` +
			`to do with a chain, since mu login pastes a token`)
	}
}

// And it is reachable, under x402, where it was put.
func TestTheKeyIsReachableUnderX402(t *testing.T) {
	src, err := os.ReadFile("x402.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `args[0] == "key"`) {
		t.Error(`mu x402 key does not reach the local key — there is then no way to ` +
			`see which key this machine signs with, or to make one`)
	}
}

// And the key command does not describe itself as a wallet, because that is
// the word that already means something else.
func TestTheKeyCommandDoesNotCallItselfAWallet(t *testing.T) {
	src, err := os.ReadFile("help.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(src), "\n") {
		if !strings.Contains(line, "mu wallet") {
			continue
		}
		// `mu wallet <method>` is the service and is fine to document. A bare
		// `mu wallet` in the help would be pointing at the old meaning.
		trimmed := strings.TrimSpace(line)
		if i := strings.Index(trimmed, "mu wallet"); i >= 0 {
			rest := strings.TrimSpace(trimmed[i+len("mu wallet"):])
			if rest == "" || strings.HasPrefix(rest, " ") {
				t.Errorf("help.go documents a bare `mu wallet`: %q — that was the "+
					"local key and is now the service", trimmed)
			}
		}
	}
}

// The key file itself does not move. A command can be renamed freely; a file
// holding real money cannot, because the rename ships, the old path is never
// read again, and somebody's funds sit at a key nothing looks for.
func TestTheKeyFilePathIsUnchanged(t *testing.T) {
	for _, f := range []string{"key.go", "agent.go", "x402pay.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), `"keys"`) &&
			!strings.Contains(string(src), `"wallet.seed"`) {
			t.Errorf("%s builds a key path that is no longer wallet.seed — renaming "+
				"the file strands whatever the old one holds", f)
		}
	}
}
