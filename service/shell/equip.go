package shell

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
// Workspaces are shared with agent commands. Never inject account credentials
// into an interactive process: other processes can read its environment.
// Users may explicitly supply credentials, but these are available to all code
// they run in that workspace.
//
// # Mounted, not baked
//
// The running binary is bind-mounted read-only rather than built into an
// image. Every box then has the version this server is running, the moment it
// is deployed, with no image to rebuild and no registry to push to. It is the
// same inode on every container, so the disk cost is nothing.

import (
	"debug/elf"
	"os"
	"strings"
	"sync"

	"mu/internal/app"
)

// muPath is where the CLI appears inside a box.
const muPath = "/usr/local/bin/mu"

// equipment is what to mount into a machine, in docker's --volume syntax.
//
// Empty when the running binary cannot be located, or when it would not run
// inside the box. Neither is a reason to refuse somebody a machine: they get
// one without the CLI, which is what they had before this existed.
func equipment() []string {
	self, err := os.Executable()
	if err != nil || strings.TrimSpace(self) == "" {
		return nil
	}
	if !static(self) {
		// Said once rather than per box, and said at all because the symptom
		// is otherwise baffling: the file is there, `ls` shows it executable,
		// and the shell answers "not found".
		notStatic.Do(func() {
			app.Log("shell", "not mounting the CLI into machines: %s is "+
				"dynamically linked and the sandbox image has no loader for it. "+
				"Build with CGO_ENABLED=0", self)
		})
		return nil
	}
	return []string{self + ":" + muPath + ":ro"}
}

var notStatic sync.Once

// static reports whether a binary will run in a container that has no C
// library.
//
// This is the difference between the mount working and being baffling. The
// sandbox image is Alpine, which has musl and no glibc loader, and a Go binary
// built the ordinary way on a glibc host is dynamically linked against one. The
// kernel then cannot start it, and the error it produces — through a shell —
// is "not found", about a file that `ls -l` has just listed. It cost an
// afternoon to read that as a missing interpreter rather than a missing file.
//
// The repository builds static everywhere it builds officially: CGO_ENABLED=0
// in the Dockerfile and in the release workflow. What is not covered is
// `go install .` by hand on a server, which is how an instance is most often
// deployed, and which produces a dynamic binary on any ordinary Linux host.
//
// An ELF with no PT_INTERP has no interpreter to be missing. That is the whole
// test, it is in the standard library, and it is cheaper than being wrong.
func static(path string) bool {
	f, err := elf.Open(path)
	if err != nil {
		// Not an ELF at all — another OS, or something we should not be
		// mounting into a Linux container regardless.
		return false
	}
	defer f.Close()
	for _, p := range f.Progs {
		if p.Type == elf.PT_INTERP {
			return false
		}
	}
	return true
}
