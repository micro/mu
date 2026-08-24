package app

// Where the log goes, and what is left on the screen.
//
// A first run printed 313 lines before it was ready. A hundred and two of them
// were the framework's own transport, broker and registry chatter — "Registry
// [memory] Registering node: video-a6deffea…" — and most of the rest was one
// service narrating an RSS fetch. The line that actually mattered, "no AI
// provider configured", was third from the top and gone before the scroll
// stopped.
//
// So an operator starting an instance for the first time could not see what was
// wrong with it, and the thing they needed to do next was invisible.
//
// The log goes to a file now. What stays on the screen is what somebody
// standing at a terminal needs: the address to open, what is configured, what
// is not, and where the rest went. Everything still reaches the file, and
// still reaches /admin/logs, so nothing is lost — it is moved off the one
// surface where volume destroys meaning.
//
// # Containers and systemd
//
// Both capture stdout and expect the log to be there, which is the opposite of
// what is wanted at a terminal. MU_LOG_STDOUT=true restores it. That is a
// choice about where this instance runs rather than about what it should say,
// which is why it is an operator's setting and not a guess made from the
// environment.

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	logMu      sync.Mutex
	logWriter  io.Writer // nil until OpenLog runs; stdout when it declines
	logPath    string
	logToStdio bool
)

// Quieten registers how to send the framework's own logger somewhere. Called
// by internal/server before OpenLog.
func Quieten(fn func(io.Writer)) { quieten = fn }

// LogPath is the file the log is being written to, or "" when it is going to
// the screen.
func LogPath() string {
	logMu.Lock()
	defer logMu.Unlock()
	return logPath
}

// OpenLog sends the log to a file and quietens the framework underneath.
//
// Called once at startup, before anything worth reading is logged. Failing to
// open the file is not fatal: an instance that will not start because it could
// not write a log is worse than a noisy one, so it falls back to the screen and
// says so.
func OpenLog() {
	logMu.Lock()
	defer logMu.Unlock()

	if truthy(os.Getenv("MU_LOG_STDOUT")) {
		logToStdio = true
		return
	}

	path := strings.TrimSpace(os.Getenv("MU_LOG_FILE"))
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			logToStdio = true
			return
		}
		path = filepath.Join(home, ".mu", "logs", "mu.log")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "could not make a log directory at %s (%v); logging to the screen\n",
			filepath.Dir(path), err)
		logToStdio = true
		return
	}
	// Appended rather than truncated: the log of the run that just crashed is
	// the one somebody wants, and starting a new one over it is how it is lost.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not open %s (%v); logging to the screen\n", path, err)
		logToStdio = true
		return
	}
	logWriter = f
	logPath = path

	// And the framework's own logger, which is where two thirds of the noise
	// came from and none of it is addressed to a person running this instance:
	// "Registry [memory] Registering node: video-a6deffea…" is a fact about a
	// transport nobody chose. Pointing it at the same file keeps it available
	// without putting it on the screen.
	//
	// Through the framework's own Init rather than slog.SetDefault, which does
	// not work and looked like it did: its logger captures a *slog.Logger when
	// the package initialises, and only falls back to slog.Default() if that
	// capture is nil. Setting the default afterwards changes a fallback that is
	// never reached, so the noise stayed exactly where it was.
	//
	// slog.SetDefault as well, for anything else that has not been handed a
	// logger.
	slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if quieten != nil {
		quieten(f)
	}
}

// quieten points the framework's logger at the log file.
//
// A function variable because internal/app may not import the framework: it is
// the bottom of the product and everything imports it, so a dependency added
// here is a dependency everywhere. internal/server fills it in, which is the
// package that already assembles everything — see hooks.go for the same
// pattern and the ledger of what it costs.
var quieten func(io.Writer)

// logDest is where a log line should be written.
func logDest() io.Writer {
	logMu.Lock()
	defer logMu.Unlock()
	if logWriter != nil && !logToStdio {
		return logWriter
	}
	return os.Stdout
}

// Announce says something to whoever is standing at the terminal, whatever the
// log is doing.
//
// For the handful of facts that are worth a person's attention at startup: the
// address, what is missing, where the log went. Deliberately separate from Log,
// because the whole point is that this is the surface volume destroys — one
// caller too many and it is the 313-line screen again.
func Announce(format string, args ...interface{}) {
	if cliMode {
		return
	}
	fmt.Printf(format+"\n", args...)
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
