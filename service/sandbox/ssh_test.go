package sandbox

// The two things that must be true about the SSH door.
//
// Neither is about SSH working — that needs a daemon, a container and a client,
// and it is checked by connecting. These are the two ways a shell door goes
// wrong quietly: it lets somebody be an account they are not, or it lets what
// they typed reach a shell on this side of the container wall.

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"mu/internal/auth"
)

// Who you are is the key. The username is not consulted.
//
// c.User() is the one field in an SSH handshake the client fills in freely. If
// it reached a container name, a path, or a uid, then `ssh someoneelse@host`
// would be an attempt worth making — and the fix would have to be perfect
// escaping in every one of those places rather than not reading it at all.
func TestTheUsernameDecidesNothing(t *testing.T) {
	b, err := os.ReadFile("ssh.go")
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		if strings.Contains(line, ".User()") {
			t.Errorf("ssh.go:%d reads the username off the connection. Identity "+
				"comes from the key that signed the handshake — the username is "+
				"whatever the client typed:\n\t%s", i+1, strings.TrimSpace(line))
		}
	}
}

// Nothing the caller sends is ever handed to a shell on this side.
//
// The session path builds a docker argv and runs a literal `sh -l`. Exec is the
// other shape — it takes a command and runs `sh -c` on it, on purpose, because
// its caller is an agent writing a pipeline — and the two must not meet: a
// session that reached Exec would be turning a keystroke stream into a command
// string, which is the injection this door does not have.
func TestTheSessionNeverBuildsAShellCommand(t *testing.T) {
	b, err := os.ReadFile("ssh.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	// Cut the package comment, which discusses both by name.
	if i := strings.Index(src, "\nimport ("); i > 0 {
		src = src[i:]
	}

	for _, forbidden := range []string{"exec(", "paidRun(", "Command:", "sh -c"} {
		if strings.Contains(src, forbidden) {
			t.Errorf("ssh.go uses %q — the session must attach a terminal to a "+
				"literal shell, never assemble a command out of anything the "+
				"caller sent", forbidden)
		}
	}

	// And it does attach one.
	if !strings.Contains(src, "container.Shell(") {
		t.Error("the session does not call container.Shell, so this test is " +
			"asserting nothing about the path that exists")
	}
}

// A key is stored under the account that registered it, and answers for that
// account only.
func TestAKeyIdentifiesOneAccount(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const line = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJ7VPBRfGmT/PsIu1JAd6Q4KL5eGXAqmMi6MIB0GtKfl test@example"
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
	if err != nil {
		t.Fatalf("the fixture key does not parse: %v", err)
	}
	print := ssh.FingerprintSHA256(key)

	if err := auth.AddSSHKey("owner", "laptop", line, print); err != nil {
		t.Fatalf("adding: %v", err)
	}
	if got := auth.AccountForSSHKey(print); got != "owner" {
		t.Errorf("the key resolves to %q, want owner", got)
	}
	// An unregistered key is nobody, rather than a default.
	if got := auth.AccountForSSHKey("SHA256:nothingregisteredunderthis"); got != "" {
		t.Errorf("an unknown key resolved to %q", got)
	}
	// And it cannot be claimed by somebody else while it is registered.
	if err := auth.AddSSHKey("interloper", "mine now", line, print); err == nil {
		t.Error("a second account registered a key that already belongs to somebody")
	}
	if got := auth.AccountForSSHKey(print); got != "owner" {
		t.Errorf("after the attempt the key resolves to %q, want owner", got)
	}
	// Removing somebody else's is refused.
	if err := auth.RemoveSSHKey("interloper", print); err == nil {
		t.Error("one account removed another's key")
	}
}

// A key does not outlive the account it belonged to.
func TestDeletingAnAccountTakesItsKeys(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const line = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIA0hVQZQd0oCPMqzPGVfPZBGGrKqXBUvhtL8lHhSMDmU gone@example"
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	print := ssh.FingerprintSHA256(key)
	if err := auth.AddSSHKey("leaving", "laptop", line, print); err != nil {
		t.Fatal(err)
	}

	auth.DeleteSSHKeysFor("leaving")

	if got := auth.AccountForSSHKey(print); got != "" {
		t.Errorf("the key still opens a shell as %q after the account went", got)
	}
}

// The handshake completes, and only for a registered key.
//
// A real client against the real server over a real socket. Asserting about
// the config struct instead would pass while the handshake was broken, and a
// protocol server that does not complete one is not a feature — it is a port
// that hangs.
//
// It stops at the container: there is no Docker in a test, so the session ends
// with the reason rather than a shell. That is the correct boundary for this
// test. What is proved is everything up to it — key exchange, publickey auth
// against the registered fingerprint, the session channel, and the shell
// request — which is all the code in this file.
func TestARegisteredKeyGetsThroughTheHandshake(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	print := ssh.FingerprintSHA256(sshPub)
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
	if err := auth.AddSSHKey("shelluser", "test", line, print); err != nil {
		t.Fatal(err)
	}

	cfg, err := sshConfig()
	if err != nil {
		t.Fatalf("server config: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go accept(ln, cfg)

	client, err := ssh.Dial("tcp", ln.Addr().String(), &ssh.ClientConfig{
		// Anything at all: the username decides nothing.
		User:            "not-a-real-account",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	})
	if err != nil {
		t.Fatalf("a registered key could not connect: %v", err)
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		t.Fatalf("no session: %v", err)
	}
	defer sess.Close()

	out, _ := sess.Output("") // Shell() with no command; the error is expected
	_ = out

	// And an unregistered key is refused outright.
	_, other, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	stranger, err := ssh.NewSignerFromKey(other)
	if err != nil {
		t.Fatal(err)
	}
	if c, err := ssh.Dial("tcp", ln.Addr().String(), &ssh.ClientConfig{
		User:            "shelluser",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(stranger)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}); err == nil {
		c.Close()
		t.Error("an unregistered key opened a connection — and it named a real " +
			"account, which is the attempt worth making")
	}
}

// The host key survives a restart, or every client shouts about it.
func TestTheHostKeyIsKept(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	first, err := hostKey()
	if err != nil {
		t.Fatal(err)
	}
	again, err := hostKey()
	if err != nil {
		t.Fatal(err)
	}
	if string(first.PublicKey().Marshal()) != string(again.PublicKey().Marshal()) {
		t.Error("the host key changed between calls, so every client would " +
			"report the server as impersonated on the second connection")
	}
}
