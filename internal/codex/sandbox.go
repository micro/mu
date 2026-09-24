package codex

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	gmai "go-micro.dev/v6/model"

	"mu/internal/dir"
	"mu/internal/persist"
	"mu/internal/settings"
)

const Version = "0.156.1"
const Model = "gpt-6-astra"

//go:embed config.toml
var config []byte

func authDir() string { return filepath.Join(dir.Root(), "codex-auth") }
func marker() string  { return filepath.Join(authDir(), "checked") }

// Checked is a cheap local readiness hint, never a substitute for the sandbox.
func Checked() bool {
	b, e := os.ReadFile(marker())
	return e == nil && string(b) == Version
}

func binary() (string, error) {
	p := settings.Get("CODEX_BINARY")
	if p == "" {
		p = "codex"
	}
	p, e := exec.LookPath(p)
	if e != nil {
		return "", fmt.Errorf("install standalone Codex %s and set CODEX_BINARY to its path", Version)
	}
	p, e = filepath.Abs(p)
	if e != nil {
		return "", e
	}
	f, e := os.Open(p)
	if e != nil {
		return "", e
	}
	var magic [4]byte
	_, e = f.Read(magic[:])
	f.Close()
	if e != nil || string(magic[:]) != "\x7fELF" {
		return "", fmt.Errorf("CODEX_BINARY must point to the native Linux Codex executable, not an npm launcher")
	}
	return p, nil
}

// No host home, data, credentials, sockets, config or environment are mounted.
// Networking remains available for OpenAI. Native tools are additionally
// disabled and environment access is removed from every thread and turn.
func sandboxArgs(binary, state string, command ...string) []string {
	a := []string{"--unshare-all", "--share-net", "--die-with-parent", "--new-session", "--cap-drop", "ALL", "--clearenv", "--ro-bind", "/usr", "/usr"}
	for _, p := range []string{"/lib", "/lib64", "/etc/ssl/certs", "/etc/resolv.conf", "/etc/hosts", "/etc/nsswitch.conf"} {
		if _, e := os.Stat(p); e == nil {
			a = append(a, "--ro-bind", p, p)
		}
	}
	a = append(a, "--symlink", "usr/bin", "/bin", "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp", "--dir", "/work", "--dir", "/home/micro", "--bind", state, "/state", "--ro-bind", binary, "/opt/codex",
		"--setenv", "HOME", "/home/micro", "--setenv", "CODEX_HOME", "/state", "--setenv", "PATH", "/usr/bin:/bin", "--setenv", "LANG", "C.UTF-8", "--chdir", "/work", "--")
	return append(a, command...)
}

func isolatedCommand(ctx context.Context, state string, command ...string) (*exec.Cmd, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("Codex preview requires Linux and Bubblewrap")
	}
	b, e := binary()
	if e != nil {
		return nil, e
	}
	bw, e := exec.LookPath("bwrap")
	if e != nil {
		return nil, fmt.Errorf("install bubblewrap before enabling Codex preview")
	}
	c := exec.CommandContext(ctx, bw, sandboxArgs(b, state, command...)...)
	c.Env = []string{"PATH=/usr/bin:/bin"}
	return c, nil
}

func temporaryState() (string, error) {
	p, e := os.MkdirTemp("", "mu-codex-")
	if e != nil {
		return "", e
	}
	if e = os.WriteFile(filepath.Join(p, "config.toml"), config, 0600); e != nil {
		os.RemoveAll(p)
		return "", e
	}
	return p, nil
}

// Check validates the actual OS boundary before a host can opt into the pilot.
// It includes a model/tool round trip and never imports a personal profile.
func Check(ctx context.Context) error {
	_ = os.Remove(marker())
	state, e := temporaryState()
	if e != nil {
		return e
	}
	defer os.RemoveAll(state)
	canary, e := os.CreateTemp("", "mu-private-canary-")
	if e != nil {
		return e
	}
	canary.Close()
	defer os.Remove(canary.Name())
	c, e := isolatedCommand(ctx, state, "/usr/bin/sh", "-c", `test ! -e "$1" && test ! -e /root && test ! -e /run && test -z "$MICRO_PRIVATE_CANARY" && test "$(ls /proc | sed -n '/^[0-9][0-9]*$/p' | wc -l)" -lt 15`, "check", canary.Name())
	if e != nil {
		return e
	}
	if e = c.Run(); e != nil {
		return fmt.Errorf("Codex sandbox check failed; Linux user namespaces must be permitted (no unrestricted fallback)")
	}
	c, e = isolatedCommand(ctx, state, "/opt/codex", "--version")
	if e != nil {
		return e
	}
	b, e := c.Output()
	if e != nil || strings.TrimSpace(string(b)) != "codex-cli "+Version {
		return fmt.Errorf("Codex preview requires tested version %s", Version)
	}
	// Verify the installed protocol exposes the isolation fields we depend on.
	c, e = isolatedCommand(ctx, state, "/opt/codex", "app-server", "generate-json-schema", "--experimental", "--out", "/state/schema")
	if e != nil {
		return e
	}
	if e = c.Run(); e != nil {
		return fmt.Errorf("could not verify Codex protocol")
	}
	schema, e := os.ReadFile(filepath.Join(state, "schema", "v2", "ThreadStartParams.json"))
	if e != nil || !strings.Contains(string(schema), `"environments"`) || !strings.Contains(string(schema), `"ephemeral"`) {
		return fmt.Errorf("Codex lacks required session isolation controls")
	}
	// A harmless dynamic tool validates the real protocol, auth and model too.
	client, close, e := start(ctx)
	if e != nil {
		return e
	}
	calls := 0
	p := provider{opts: gmai.NewOptions(gmai.WithModel(Model), gmai.WithToolHandler(func(_ context.Context, c gmai.ToolCall) gmai.ToolResult {
		calls++
		return gmai.ToolResult{ID: c.ID, Content: "MICRO_CODEX_OK"}
	}))}
	response, e := p.generate(ctx, client, &gmai.Request{SystemPrompt: "Follow the user's instruction exactly.", Prompt: "Call micro_probe once, then reply with exactly the text it returns.", Tools: []gmai.Tool{{Name: "micro_probe", Description: "Return the installation check result.", Properties: map[string]any{"values": map[string]any{"type": "array", "description": "Optional test values; leave empty."}}}}})
	close()
	if e != nil {
		return e
	}
	if strings.TrimSpace(response.Reply) != "MICRO_CODEX_OK" || calls != 1 {
		return fmt.Errorf("Codex response check failed")
	}
	if e = os.MkdirAll(authDir(), 0700); e != nil {
		return e
	}
	return persist.WritePath(marker(), []byte(Version))
}

// Login creates a separate credential store. No personal Codex files are copied.
func Login(ctx context.Context) error {
	unlock, e := lock(ctx)
	if e != nil {
		return e
	}
	defer unlock()
	_ = os.Remove(marker())
	b, e := binary()
	if e != nil {
		return e
	}
	if e = os.MkdirAll(authDir(), 0700); e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(authDir(), "config.toml"), config, 0600); e != nil {
		return e
	}
	c := exec.CommandContext(ctx, b, "login", "--device-auth")
	c.Dir = authDir()
	c.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + authDir(), "CODEX_HOME=" + authDir(), "LANG=C.UTF-8"}
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

func start(ctx context.Context) (*rpc, func(), error) {
	unlock, e := lock(ctx)
	if e != nil {
		return nil, nil, e
	}
	state, e := temporaryState()
	if e != nil {
		unlock()
		return nil, nil, e
	}
	cleanup := func() { os.RemoveAll(state); unlock() }
	version, e := isolatedCommand(ctx, state, "/opt/codex", "--version")
	if e != nil {
		cleanup()
		return nil, nil, e
	}
	v, e := version.Output()
	if e != nil || strings.TrimSpace(string(v)) != "codex-cli "+Version {
		cleanup()
		return nil, nil, fmt.Errorf("Codex sandbox unavailable or version changed; run mu codex check")
	}
	auth, e := os.ReadFile(filepath.Join(authDir(), "auth.json"))
	if e != nil {
		cleanup()
		return nil, nil, fmt.Errorf("sign in to Micro's isolated Codex profile with mu codex login")
	}
	if e = os.WriteFile(filepath.Join(state, "auth.json"), auth, 0600); e != nil {
		cleanup()
		return nil, nil, e
	}
	ctx, cancel := context.WithCancel(ctx)
	c, e := isolatedCommand(ctx, state, "/opt/codex", "app-server", "--listen", "stdio://")
	if e != nil {
		cancel()
		cleanup()
		return nil, nil, e
	}
	c.WaitDelay = time.Second
	in, e := c.StdinPipe()
	if e != nil {
		cancel()
		cleanup()
		return nil, nil, e
	}
	out, e := c.StdoutPipe()
	if e != nil {
		in.Close()
		cancel()
		cleanup()
		return nil, nil, e
	}
	if e = c.Start(); e != nil {
		in.Close()
		cancel()
		cleanup()
		return nil, nil, e
	}
	close := func() {
		in.Close()
		cancel()
		c.Wait()
		// Persist only rotated credentials; never retain a session or memory.
		if updated, e := os.ReadFile(filepath.Join(state, "auth.json")); e == nil && len(updated) > 0 {
			_ = persist.WritePath(filepath.Join(authDir(), "auth.json"), updated)
		}
		cleanup()
	}
	return newRPC(out, in), close, nil
}
