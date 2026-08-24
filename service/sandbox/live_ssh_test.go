package sandbox

// The whole chain, live: an SSH client, a real handshake, a real container,
// the CLI on the path, and a credential only this session has.
//
// Skipped where there is no Docker.

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"mu/internal/auth"
	"mu/internal/container"
)

func TestLiveShellThroughSSH(t *testing.T) {
	if !container.Available() {
		t.Skip("no docker: " + container.Reason())
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MU_DOMAIN", "live.test")
	t.Setenv("SANDBOX_MEMORY", "256m")
	t.Setenv("SANDBOX_CPUS", "0.5")

	const who = "sshlive"
	if err := auth.Create(&auth.Account{ID: who, Name: "SSH Live", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { DeleteMachine(who) })

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, _ := ssh.NewSignerFromKey(priv)
	sshPub, _ := ssh.NewPublicKey(pub)
	if err := auth.AddSSHKey(who, "live",
		strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))),
		ssh.FingerprintSHA256(sshPub)); err != nil {
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
		User: "whatever", Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 15 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if err := sess.RequestPty("xterm", 40, 100, ssh.TerminalModes{}); err != nil {
		t.Fatalf("pty: %v", err)
	}
	in, err := sess.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	sess.Stdout, sess.Stderr = &out, &out
	if err := sess.Shell(); err != nil {
		t.Fatalf("shell: %v", err)
	}

	// No sentinel words in the input. A pty echoes what is typed, so the
	// command line comes back in the output and any marker in it matches
	// itself — which is how the first version of this test reported that the
	// CLI was missing while the very next line showed its path.
	io.WriteString(in, "id -u; pwd; command -v mu; "+ //nolint:errcheck
		"echo URL=$MU_URL; echo TOK=${MU_TOKEN:+present}; exit\n")

	done := make(chan error, 1)
	go func() { done <- sess.Wait() }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("the shell never exited")
	}

	got := out.String()
	t.Logf("session said:\n%s", got)

	if !strings.Contains(got, "/work") {
		t.Errorf("the shell did not start in the account's directory:\n%s", got)
	}
	if !strings.Contains(got, "URL=https://live.test") {
		t.Errorf("MU_URL did not reach the session:\n%s", got)
	}
	if !strings.Contains(got, "TOK=present") {
		t.Errorf("no credential reached the session:\n%s", got)
	}
	// The CLI is only there when the running binary could be mounted — see
	// equipment(). A dynamically linked build is not, which is a property of
	// how this test binary was compiled rather than of the code under test.
	//
	// Asserted on where `command -v` resolved it, not on a marker: the echoed
	// input is in this output too.
	if len(equipment()) > 0 && !strings.Contains(got, muPath+"\n") &&
		!strings.Contains(got, muPath+"\r") {
		t.Errorf("the CLI was mounted but did not resolve on the path:\n%s", got)
	}
}
