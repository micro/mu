package shell

// The sandbox, over SSH.
//
// A sandbox you can only reach through a tool call is a build server with no
// terminal. The thing people actually want from a box is to be in it — to run
// something, read the error, fix it, run it again — and that loop is the one
// shape a request/response tool cannot express.
//
// So this is a door, in the same sense /mcp is a door onto the tools and IMAP
// is a door onto the mailbox. service/mail runs its own IMAP server in this
// process rather than shelling out to dovecot, and this is the same decision
// for the same reason: the protocol is how the caller already speaks, and
// everything behind it is the code that was already there.
//
// # Mu is the SSH server. The container is not.
//
// No sshd runs inside a box. Nothing distributes keys into one. This process
// answers the connection, authenticates it against keys registered on an
// account, and only then attaches the session to that account's container —
// which means every guarantee the sandbox already had still holds, unchanged
// and without being restated here: no capabilities, no new privileges, the
// memory, CPU and PID caps, no swap, and whatever network the box was given.
//
// It also means there is nothing to break into on the way. A container with an
// sshd in it has a listening service, a key, a user database and a password
// path; this one has a process that is asleep until somebody who already
// proved who they are is attached to it.
//
// # Identity is the key, not the username
//
// `ssh anything@host` works and the "anything" is ignored. Who you are is
// decided by which registered public key signed the handshake, and nothing
// downstream reads the username — because the username is the one part of an
// SSH handshake an attacker chooses freely, and every path where it reaches a
// container name, a path or a command is a way to be somebody else.
//
// # There is no command to inject
//
// Worth stating plainly, because it is the thing that goes wrong in software
// shaped like this. Nothing the caller sends is ever interpolated into a
// string that a shell then parses. The docker invocation is an argv — see
// container.Run.argv, which this shares with every other exec — the container
// name comes from slug(), the user is an integer, and the command is the
// literal "sh -l". Keystrokes go to the terminal of that shell, inside the
// box, which is exactly where they are supposed to go and nowhere else.
//
// Exec is the opposite and deliberately so: it takes a command and runs
// `sh -c` on it, because the caller is an agent writing a pipeline. That is a
// shell in a box with no capabilities and no route back here. This path does
// not use it.
//
// # Off unless somebody turns it on
//
// SHELL_SSH_PORT is unset by default and then nothing listens. An operator
// who wants it picks the port, which is a decision rather than a default: 22
// on the host is the host's own sshd, and taking it by accident is how a
// deploy locks somebody out of their own machine.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/container"
	"mu/internal/data"
	store "mu/internal/files"
	"mu/internal/sshaccess"
)

// LoadSSH starts the SSH door if this instance has been given a port.
func LoadSSH() {
	addr := sshaccess.Port()
	if addr == "" || strings.EqualFold(addr, "off") {
		return
	}
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}

	cfg, err := sshConfig()
	if err != nil {
		app.Log("shell", "no ssh: %v", err)
		return
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		app.Log("shell", "ssh cannot listen on %s: %v", addr, err)
		return
	}
	app.Log("shell", "ssh on %s", addr)
	go accept(ln, cfg)
}

// accept takes connections until the listener is closed.
//
// Split out from LoadSSH so a test can drive the real server over a real
// socket with a real client. The alternative is asserting about the config
// struct, which would pass while the handshake was broken — and a protocol
// server that does not complete a handshake is not a feature, it is a port
// that hangs.
func accept(ln net.Listener, cfg *ssh.ServerConfig) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go serveSSH(conn, cfg)
	}
}

// sshConfig is the server's configuration: how it identifies itself, and who
// it will let in.
func sshConfig() (*ssh.ServerConfig, error) {
	signer, err := hostKey()
	if err != nil {
		return nil, err
	}

	cfg := &ssh.ServerConfig{
		// Keys only. No password callback at all — not one that always
		// refuses, none — so there is no path where a guess is even scored,
		// and nothing to rate-limit.
		PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			who := auth.AccountForSSHKey(ssh.FingerprintSHA256(key))
			if who == "" {
				return nil, fmt.Errorf("unknown key")
			}
			// The account travels in the permissions rather than being read
			// back off the connection later. c.User() is whatever the client
			// typed and is never consulted again.
			return &ssh.Permissions{Extensions: map[string]string{"account": who}}, nil
		},
		// Long enough for a laptop on a bad connection, short enough that a
		// half-open handshake is not a way to hold resources.
		AuthLogCallback: func(c ssh.ConnMetadata, method string, err error) {
			if err != nil {
				app.Log("shell", "ssh refused %s from %s", method, c.RemoteAddr())
			}
		},
	}
	cfg.AddHostKey(signer)
	return cfg, nil
}

// hostKey is this instance's SSH identity, made once and kept.
//
// Generated rather than configured, because an operator should not have to
// produce one to switch this on — and kept, because a host key that changes on
// restart is the warning every SSH client shouts about, and teaching people to
// ignore that warning is worse than not running the server.
func hostKey() (ssh.Signer, error) {
	if b, err := data.LoadFile(hostKeyFile); err == nil && len(b) > 0 {
		return ssh.ParsePrivateKey(b)
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	b, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		return nil, err
	}
	if err := data.SaveFile(hostKeyFile, string(pem.EncodeToMemory(b))); err != nil {
		return nil, err
	}
	return ssh.NewSignerFromKey(priv)
}

const hostKeyFile = "ssh_host_key"

// serveSSH handles one connection.
func serveSSH(nc net.Conn, cfg *ssh.ServerConfig) {
	defer nc.Close()

	// A handshake that never completes must not hold a goroutine and a socket
	// for as long as the peer feels like.
	nc.SetDeadline(time.Now().Add(handshakeWait)) //nolint:errcheck
	conn, chans, reqs, err := ssh.NewServerConn(nc, cfg)
	if err != nil {
		return
	}
	defer conn.Close()
	nc.SetDeadline(time.Time{}) //nolint:errcheck

	who := conn.Permissions.Extensions["account"]
	if who == "" {
		return
	}

	go ssh.DiscardRequests(reqs)
	for ch := range chans {
		if ch.ChannelType() != "session" {
			ch.Reject(ssh.UnknownChannelType, "only sessions") //nolint:errcheck
			continue
		}
		go session(ch, who)
	}
}

const handshakeWait = 30 * time.Second

// session is one authenticated SSH session: either a shell in the account's
// container, or SFTP over the account's Files store.
func session(newCh ssh.NewChannel, accountID string) {
	ch, reqs, err := newCh.Accept()
	if err != nil {
		return
	}
	defer ch.Close()

	// What the client asks for, and what is answered.
	//
	// A shell and the sftp subsystem are agreed to. Exec and every kind of
	// forwarding remain refused. SFTP is handled before any container path is
	// touched: the SSH key proves the account, and Files authorises everything
	// after that.
	wants := make(chan string, 1)
	var size window
	var resized *container.Session
	go func() {
		for req := range reqs {
			switch req.Type {
			case "pty-req":
				// The payload is TERM, then the size. The size is what is
				// wanted; a shell started at the default 80x24 when the client
				// said otherwise draws everything in the wrong place until the
				// first resize.
				size = ptySize(req.Payload)
				req.Reply(true, nil) //nolint:errcheck
			case "shell":
				req.Reply(true, nil) //nolint:errcheck
				wants <- "shell"
			case "subsystem":
				var asked struct{ Name string }
				if err := ssh.Unmarshal(req.Payload, &asked); err != nil || asked.Name != "sftp" {
					req.Reply(false, nil) //nolint:errcheck
					continue
				}
				req.Reply(true, nil) //nolint:errcheck
				wants <- "sftp"
			case "window-change":
				w := changeSize(req.Payload)
				size = w
				if resized != nil {
					resized.Resize(w.rows, w.cols)
				}
				req.Reply(true, nil) //nolint:errcheck
			default:
				req.Reply(false, nil) //nolint:errcheck
			}
		}
	}()

	var requested string
	select {
	case requested = <-wants:
	case <-time.After(shellWait):
		fmt.Fprint(ch, "no shell or sftp subsystem was asked for\r\n")
		return
	}
	if requested == "sftp" {
		app.Log("files", "sftp session for %s", accountID)
		_ = store.ServeSFTP(accountID, ch)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), sessionLimit)
	defer cancel()

	if err := ready(ctx, accountID); err != nil {
		fmt.Fprintf(ch, "%s\r\n", err)
		return
	}

	// A credential for this session and nothing else.
	//
	// Minted here, carried in this exec's environment, revoked below when the
	// shell exits. Never written into the volume, and never given to an
	// agent's command in the same box — see equip.go for why that line is the
	// whole security argument.
	token, tokenID := sessionToken(accountID)
	defer revoke(accountID, tokenID)

	sh, err := container.Shell(ctx, container.Run{
		Name: machineFor(accountID),
		Dir:  home(accountID),
		User: runAs(accountID),
		Env:  sessionEnv(token),
	}, ch, ch)
	if err != nil {
		// Said properly, because this is the entire session. Somebody who has
		// just authenticated over SSH and been hung up on with one clause of a
		// sentence has no way to tell whether they typed something wrong,
		// whether their account lacks something, or whether the instance
		// simply cannot do this — and it is the last one, always.
		fmt.Fprintf(ch, "No shell here: %s.\r\n\r\n", err)
		fmt.Fprint(ch, "A shell runs in a container of your own, so the instance "+
			"needs a container runtime.\r\nThe operator installs Docker and restarts "+
			"the server; nothing on your side will change this.\r\n\r\n")
		fmt.Fprint(ch, "Everything else still works: the web app, the agent, "+
			"mail, and every tool over MCP.\r\n")
		return
	}
	// The size the client asked for, and every change after it.
	sh.Resize(size.rows, size.cols)
	resized = sh

	app.Log("shell", "ssh session for %s", accountID)
	err = sh.Wait()
	// The exit status, so the client's own shell sees what happened rather
	// than treating every disconnection as success.
	code := 0
	if err != nil {
		code = 1
	}
	ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{uint32(code)})) //nolint:errcheck
}

const (
	// shellWait is how long to wait for the client to ask for a shell after
	// opening the channel. It is the first thing every client does.
	shellWait = 15 * time.Second

	// sessionLimit is the longest a single session may last.
	//
	// A shell holds a container open, and a container is the expensive thing
	// here — the reaper exists because idle boxes are what fill a small
	// machine. This is a ceiling rather than an idle timeout: the difference
	// matters, because somebody watching a long build is not idle and should
	// not be cut off for being quiet.
	sessionLimit = 4 * time.Hour
)

// window is a terminal's size in characters.
type window struct{ rows, cols uint16 }

// ptySize reads the size out of a pty-req payload.
//
// The wire format is a string (TERM), then four uint32s: columns, rows, and
// the size in pixels, which nothing uses. Sizes arrive as 32-bit and are used
// as 16-bit because that is what the ioctl takes — a terminal claiming more
// than 65535 columns is not one worth accommodating.
func ptySize(payload []byte) window {
	if len(payload) < 4 {
		return window{}
	}
	n := int(be32(payload))
	if len(payload) < 4+n+8 {
		return window{}
	}
	rest := payload[4+n:]
	return window{cols: uint16(be32(rest)), rows: uint16(be32(rest[4:]))}
}

// changeSize reads the size out of a window-change payload, which is the same
// four numbers with no string in front.
func changeSize(payload []byte) window {
	if len(payload) < 8 {
		return window{}
	}
	return window{cols: uint16(be32(payload)), rows: uint16(be32(payload[4:]))}
}

func be32(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

// sessionToken mints a credential that lives as long as this session.
//
// Named for what it is, so an account looking at /token sees "ssh session"
// rather than a mystery — and since it is revoked on disconnect, that list
// shows the sessions open right now, which is a better answer than a
// permanent token nobody remembers making.
//
// Expiry as well as revocation, because revocation is code that has to run: a
// process killed mid-session never reaches the defer, and a token with no
// expiry would then outlive the shell it was made for.
func sessionToken(accountID string) (raw, id string) {
	t, secret, err := auth.CreateToken(accountID, "ssh session", nil,
		time.Now().Add(sessionLimit))
	if err != nil {
		// A shell with no credential still works — the CLI is on the path and
		// says it is not signed in. Better than refusing the session.
		app.Log("shell", "no session token for %s: %v", accountID, err)
		return "", ""
	}
	return secret, t.ID
}

// revoke takes the session's credential away when the shell exits.
func revoke(accountID, tokenID string) {
	if tokenID == "" {
		return
	}
	if err := auth.DeleteToken(tokenID, accountID); err != nil {
		app.Log("shell", "could not revoke the session token for %s: %v", accountID, err)
	}
}
