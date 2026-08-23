package sandbox

// What is in the box besides a shell.
//
// A machine with nothing in it is a machine. What makes this one worth having
// is that the instance's own command line is on the path: `mu news list`,
// `mu inbox`, `mu ask` — every tool this server has, in a place where they
// compose with pipes, redirection, loops and everything else a shell already
// knows how to do. No other door can do that. MCP hands a model one call at a
// time; HTTP hands a program the glue to write; a shell hands you a language
// for putting things together that everybody already speaks.
//
// # Capability by default, credential by authentication
//
// The binary goes in every box, always. It is harmless on its own — a CLI with
// no URL and no token cannot reach anything, so mounting it costs nothing and
// hiding it behind a setting would only mean nobody ever finds it.
//
// The credential does not. It is minted for one interactive session, passed in
// that exec's environment, and revoked when the session ends. It is never
// written to the volume and never given to an agent's command.
//
// That line is the whole security argument. Today the sandbox is safe because
// it holds nothing: no capabilities, tight limits, and no secrets. An agent
// runs code in there that it fetched off the internet, and the reason that is
// acceptable is that there is nothing to steal. A token sitting in the box
// would end that — one `cat` and a `curl` in a script the agent was talked
// into running, and the account's API access is gone.
//
// An SSH session is different in the one way that matters: a person
// authenticated with a key, for a bounded time. That is what earns a
// credential, and it expires with the session that earned it.
//
// # Mounted, not baked
//
// The running binary is bind-mounted read-only rather than built into an
// image. Every box then has the version this server is running, the moment it
// is deployed, with no image to rebuild and no registry to push to. It is the
// same inode on every container, so the disk cost is nothing.

import (
	"os"
	"strings"

	"mu/internal/origin"
)

// muPath is where the CLI appears inside a box.
const muPath = "/usr/local/bin/mu"

// equipment is what to mount into a machine, in docker's --volume syntax.
//
// Empty when the running binary cannot be located, which is a real state — a
// process whose executable has been replaced or deleted underneath it — and
// not a reason to refuse somebody a machine. They get a box without the CLI,
// which is what they had before this existed.
func equipment() []string {
	self, err := os.Executable()
	if err != nil || strings.TrimSpace(self) == "" {
		return nil
	}
	return []string{self + ":" + muPath + ":ro"}
}

// session is the environment one interactive shell gets.
//
// Both halves are needed or neither is useful: the CLI reads MU_URL and
// MU_TOKEN from the environment — see internal/cli/config.go — so this is the
// entire integration, and there is no CLI change anywhere in this feature.
//
// Empty when this instance does not know its own public address. A token
// without somewhere to send it is a credential handed out for nothing.
func sessionEnv(token string) map[string]string {
	at := strings.TrimSuffix(origin.Self(), "/")
	if at == "" || token == "" {
		return nil
	}
	return map[string]string{"MU_URL": at, "MU_TOKEN": token}
}
