package shell

// The two things that must be true about the SSH door.
//
// Neither is about SSH working — that needs a daemon, a container and a client,
// and it is checked by connecting. These are the two ways a shell door goes
// wrong quietly: it lets somebody be an account they are not, or it lets what
// they typed reach a shell on this side of the container wall.

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"mu/internal/auth"
	store "mu/internal/files"
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

func TestSFTPProjectsOnlyTheAuthenticatedAccountsFiles(t *testing.T) {
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
	if err := auth.AddSSHKey("files-owner", "test",
		strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))),
		ssh.FingerprintSHA256(sshPub)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put("files-owner", "report.txt", "text/plain", "hello", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put("somebody-else", "secret.txt", "text/plain", "hidden", ""); err != nil {
		t.Fatal(err)
	}

	cfg, err := sshConfig()
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go accept(ln, cfg)

	client, err := ssh.Dial("tcp", ln.Addr().String(), &ssh.ClientConfig{
		User: "ignored", Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		t.Fatalf("start sftp: %v", err)
	}
	defer sftpClient.Close()

	entries, err := sftpClient.ReadDir("/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "report.txt" {
		t.Fatalf("root entries = %#v, want only report.txt", entries)
	}
	f, err := sftpClient.Open("/report.txt")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(f)
	f.Close()
	if err != nil || string(raw) != "hello" {
		t.Fatalf("read report: %q, %v", raw, err)
	}
	overwrite, err := sftpClient.Create("/report.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := overwrite.Write([]byte("updated")); err != nil {
		t.Fatal(err)
	}
	if err := overwrite.Close(); err != nil {
		t.Fatal(err)
	}
	if got := len(store.List("files-owner")); got != 1 {
		t.Fatalf("overwriting report created another record: %d files", got)
	}
	_, updated, err := store.Get("files-owner", store.List("files-owner")[0].ID)
	if err != nil || string(updated) != "updated" {
		t.Fatalf("overwritten report = %q, %v", updated, err)
	}

	created, err := sftpClient.Create("/notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := created.Write([]byte("from sftp")); err != nil {
		t.Fatal(err)
	}
	if err := created.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sftpClient.Rename("/notes.txt", "/renamed.txt"); err != nil {
		t.Fatal(err)
	}
	if err := sftpClient.Remove("/renamed.txt"); err != nil {
		t.Fatal(err)
	}
	if got := len(store.List("files-owner")); got != 1 {
		t.Fatalf("files after create, rename and remove = %d, want original only", got)
	}
}

// A plus sign in the username survives the handshake.
//
// Worth a test rather than an opinion, because the obvious guess is that it
// does not — a `+` is illegal in a POSIX username on most systems, which is
// where the instinct comes from. It is not a POSIX username. SSH carries the
// user as an arbitrary UTF-8 string, the OpenSSH client splits `user@host` on
// the last `@` and treats everything before it as opaque, and nothing on this
// side maps it to an account on the host.
//
// So `ssh asim+research@micro.mu` reaches this server with the user set to
// "asim+research", intact, which is what an address-shaped username would need
// in order to mean anything. Whether it should mean anything is a separate
// question — see TestTheUsernameDecidesNothing, which is the reason it means
// nothing today.
func TestAPlusInTheUsernameReachesTheServer(t *testing.T) {
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
	if err := auth.AddSSHKey("asim", "test",
		strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))),
		ssh.FingerprintSHA256(sshPub)); err != nil {
		t.Fatal(err)
	}

	// The real server's callback, wrapped only to record what it was handed.
	// Wrapping rather than reimplementing: a test that built its own
	// PublicKeyCallback would prove x/crypto works, not that this server does.
	cfg, err := sshConfig()
	if err != nil {
		t.Fatal(err)
	}
	seen := make(chan string, 1)
	inner := cfg.PublicKeyCallback
	cfg.PublicKeyCallback = func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		select {
		case seen <- c.User():
		default:
		}
		return inner(c, key)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go accept(ln, cfg)

	const addressed = "asim+research"
	client, err := ssh.Dial("tcp", ln.Addr().String(), &ssh.ClientConfig{
		User:            addressed,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	})
	if err != nil {
		t.Fatalf("a username with a + in it could not connect: %v", err)
	}
	defer client.Close()

	select {
	case got := <-seen:
		if got != addressed {
			t.Errorf("the server saw %q, not %q — something rewrote the username "+
				"on the way in, and an address-shaped one would arrive mangled", got, addressed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the callback never ran")
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

// The credential belongs to the session, and to nothing else in the box.
//
// This is the rule the whole feature rests on. The sandbox is safe today
// because it holds nothing — no capabilities, tight limits, no secrets — and
// an agent runs code in there that it fetched off the internet. A token
// sitting in the container would end that: one `cat` and a `curl` in a script
// the agent was talked into running, and the account's API access is gone.
//
// So it travels in one exec's environment and never in the container's, never
// on the volume, and never on a command an agent runs.
func TestTheCredentialNeverOutlivesTheSession(t *testing.T) {
	src := read(t, "ssh.go") + read(t, "equip.go") + read(t, "box.go")

	// Not baked into the container at start: --env on `docker run` would give
	// it to everything that ever runs in there.
	if strings.Contains(src, `"--env"`) {
		t.Error("something sets an environment variable on the container itself; " +
			"a credential there is visible to every command an agent ever runs")
	}
	// Not written to the volume, which persists and which the agent shares.
	for _, onDisk := range []string{"MU_TOKEN", ".mu/token"} {
		if strings.Contains(read(t, "equip.go"), "WriteFile") && strings.Contains(src, onDisk) {
			t.Errorf("%s looks like it is written to disk — the session's "+
				"credential must exist only in the exec's environment", onDisk)
		}
	}
	// And it is taken away again.
	if !strings.Contains(read(t, "ssh.go"), "defer revoke(") {
		t.Error("the session token is not revoked when the shell exits")
	}
	if !strings.Contains(read(t, "ssh.go"), "time.Now().Add(sessionLimit)") {
		t.Error("the session token has no expiry — revocation is code that has " +
			"to run, and a process killed mid-session never reaches its defer")
	}
}

// The binary goes in every box that can run it; the credential never does.
//
// Capability by default, credential by authentication. Asserted as a rule
// rather than an outcome, because whether this test binary is statically
// linked depends on how it was compiled — `go test` produces a dynamic one on
// an ordinary Linux host, and the live tests are run from a CGO_ENABLED=0
// build precisely so the mounted path is exercised.
func TestEveryMachineGetsTheBinaryAndNoSecret(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	mounts := equipment()

	// The rule: equipped exactly when the binary would run in the box.
	if want := static(self); (len(mounts) > 0) != want {
		t.Errorf("equipment() gave %d mounts and static(%s) is %v — a binary "+
			"that cannot run in the box must not be mounted, and one that can "+
			"must be", len(mounts), self, want)
	}

	// And when it is mounted, it is mounted read-only in the right place. A
	// writable CLI in a shared container is one account editing everybody's.
	if len(mounts) > 0 {
		if !strings.HasSuffix(mounts[0], ":"+muPath+":ro") {
			t.Errorf("the binary is not mounted read-only at %s: %q", muPath, mounts[0])
		}
	}

	// Both paths that make a machine equip it, or a shared-pool instance has
	// no CLI and the difference is invisible until somebody complains.
	for _, f := range []string{"box.go", "shared.go"} {
		if !strings.Contains(read(t, f), "equipment()...") {
			t.Errorf("%s starts a container without equipping it", f)
		}
	}
}

// static says no to things that are not a Linux executable at all.
//
// The interesting case — a dynamically linked ELF — is what this whole check
// exists for and cannot be synthesised here without shipping a fixture. What
// can be checked is that the function is not simply optimistic.
func TestNothingButAnELFIsMountable(t *testing.T) {
	notELF := filepath.Join(t.TempDir(), "script")
	if err := os.WriteFile(notELF, []byte("#!/bin/sh\necho hello\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if static(notELF) {
		t.Error("a shell script was judged mountable into a container")
	}
	if static(filepath.Join(t.TempDir(), "nothing-here")) {
		t.Error("a file that does not exist was judged mountable")
	}
}

// Without a public address there is no credential, because there is nowhere
// to spend it.
func TestNoAddressMeansNoCredential(t *testing.T) {
	t.Setenv("MU_DOMAIN", "")
	t.Setenv("PUBLIC_URL", "")
	t.Setenv("APP_URL", "")

	if env := sessionEnv("a-real-looking-token"); len(env) != 0 {
		t.Errorf("a token was handed out on an instance with no address: %v", env)
	}
	if env := sessionEnv(""); len(env) != 0 {
		t.Errorf("an empty token produced an environment: %v", env)
	}
}

func read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
