package container

// A terminal, for a session somebody is sitting at.
//
// `docker exec -t` calls isatty() on its own stdin and refuses anything else:
// "the input device is not a TTY". An SSH channel is a pipe, so handing it
// straight to docker fails — which is exactly what happened the first time the
// SSH door was connected to.
//
// So a pty pair is opened here and docker is given the slave. It then sees a
// real terminal, allocates one inside the container, and the master end is
// what the session copies bytes to and from. That also makes a window resize
// expressible, which a pipe cannot carry: the size is an ioctl on the master,
// not data in the stream.
//
// Linux only, and deliberately not abstracted. The container runtime is
// Docker on Linux; a portable pty layer would be a package to maintain for a
// platform this never runs on.

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// pty is the two ends of one terminal.
type pty struct {
	master *os.File // what we read and write, and resize
	slave  *os.File // what the child is given
}

// openPTY makes a terminal pair.
//
// The dance is the POSIX one, done with ioctls because Go has no wrapper:
// open the multiplexer, unlock the slave, ask which number it got, open it.
// Skipping the unlock is the classic bug — the slave opens with EIO and the
// symptom is a shell that exits immediately with nothing printed.
func openPTY() (*pty, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, fmt.Errorf("no terminal available: %w", err)
	}

	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		master.Close()
		return nil, fmt.Errorf("could not unlock the terminal: %w", err)
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		master.Close()
		return nil, fmt.Errorf("could not name the terminal: %w", err)
	}

	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, fmt.Errorf("could not open the terminal: %w", err)
	}
	return &pty{master: master, slave: slave}, nil
}

// resize tells the terminal how big the window is.
//
// Sent on the master, which is what makes SIGWINCH reach the program inside —
// so a full-screen editor redraws when somebody drags the window, rather than
// staying the size it was when the session opened. Ignored errors: a resize
// that fails is a cosmetic problem and never a reason to drop a session.
func (p *pty) resize(rows, cols uint16) {
	if p == nil || p.master == nil {
		return
	}
	unix.IoctlSetWinsize(int(p.master.Fd()), unix.TIOCSWINSZ, //nolint:errcheck
		&unix.Winsize{Row: rows, Col: cols})
}

// close releases both ends.
func (p *pty) close() {
	if p == nil {
		return
	}
	if p.slave != nil {
		p.slave.Close()
	}
	if p.master != nil {
		p.master.Close()
	}
}
