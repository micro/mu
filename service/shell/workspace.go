package shell

// What is on somebody's machine, for a page that has to show it.
//
// Exported for the same reason apps.AuthoredBy is: the page that composes a
// machine, an agent and somewhere to put what it builds is not in this package
// and must not be — see agent/code. What it needs is the answer, not the means
// of getting one. Every rule about where an account's files live and which
// container holds them is in box.go and stays there; handing those out so a
// caller could do this itself is how a second, subtly different copy of
// home() and machineFor() comes to exist.
//
// # It does not wake the machine
//
// Which is the whole design of it. Listing files is a read, and a read that
// starts a Debian container has a cost nobody asked for: memory on this box,
// and on a small one an eviction — room() stops the machine somebody else is
// not using, and "somebody opened a page" is not a good enough reason to take
// another person's away. So a machine that is asleep reports itself asleep and
// the page says so. The files are safe in the volume either way; they come back
// the moment the account asks for something.
//
// # It does not charge
//
// Nothing ran on the caller's behalf. shell_run is priced because a command is
// work the caller asked for; a directory listing rendered to draw a page is
// this instance looking at its own state, and billing for it would make opening
// a page cost money. See paidRun for the other half of that rule.

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"mu/internal/container"
)

// A File is one entry at the top of an account's workspace.
type File struct {
	Name string
	Size int64
	Dir  bool
}

// A Workspace is what an account has on its machine, as far as a page can see
// without waking it.
type Workspace struct {
	// Awake says whether the machine was up to be asked. False means unknown
	// rather than empty, and a page must say which — "no files" and "asleep"
	// are different sentences and only one of them is alarming.
	Awake bool
	// Home is where the files are: /work on a machine of your own, and
	// /work/<you> in a shared pool.
	Home string
	// Files is the top level, directories first, capped. Total is how many
	// there were before the cap.
	Files []File
	Total int
}

// workspaceShown is how many entries a page is given. A rail is 250px wide and
// a list that runs off the bottom of it is not more informative than one that
// says how many it did not draw.
const workspaceShown = 40

// WorkspaceOf is the top level of an account's workspace.
//
// An error is this instance failing to look — a runtime that went away, a
// container that would not answer — and is worth showing. An account with no
// machine, or one asleep, is not an error: it is the ordinary state of a
// machine nobody has used today.
func WorkspaceOf(ctx context.Context, accountID string) (Workspace, error) {
	ws := Workspace{Home: home(accountID)}
	if strings.TrimSpace(accountID) == "" || !Configured() {
		return ws, nil
	}

	name := machineFor(accountID)
	up, err := container.Running(ctx, name)
	if err != nil {
		return ws, err
	}
	if !listed(up, name) {
		return ws, nil
	}
	ws.Awake = true

	// find rather than ls, because ls is for reading and this is for parsing:
	// -printf gives exactly three fields in a fixed order with a tab between
	// them, where ls -l gives a format that changes with the locale, the width
	// of the columns and whether the file is recent enough to show a time.
	//
	// One level. A workspace with a node_modules in it has a hundred thousand
	// files below this and none of them are what somebody is looking for.
	res, err := container.Exec(ctx, container.Run{
		Name:    name,
		Shell:   "bash",
		Command: `find . -mindepth 1 -maxdepth 1 -printf '%y\t%s\t%f\n'`,
		Dir:     ws.Home,
		User:    runAs(accountID),
		Wait:    quickWait,
	})
	if err != nil {
		return ws, err
	}
	// A non-zero exit with no output is an empty or missing directory, which is
	// what a machine that has been started and never written to looks like.
	if res.Code != 0 && strings.TrimSpace(res.Out) == "" {
		return ws, nil
	}

	for _, line := range strings.Split(res.Out, "\n") {
		parts := strings.SplitN(strings.TrimRight(line, "\r"), "\t", 3)
		if len(parts) != 3 || parts[2] == "" {
			continue
		}
		size, _ := strconv.ParseInt(parts[1], 10, 64)
		ws.Files = append(ws.Files, File{Name: parts[2], Size: size, Dir: parts[0] == "d"})
	}

	// Directories first, then alphabetical: the order somebody reads a
	// directory in, and one that does not depend on what find happened to
	// return first.
	sort.SliceStable(ws.Files, func(i, j int) bool {
		if ws.Files[i].Dir != ws.Files[j].Dir {
			return ws.Files[i].Dir
		}
		return strings.ToLower(ws.Files[i].Name) < strings.ToLower(ws.Files[j].Name)
	})
	ws.Total = len(ws.Files)
	if len(ws.Files) > workspaceShown {
		ws.Files = ws.Files[:workspaceShown]
	}
	return ws, nil
}

// ReadFile is one file out of an account's workspace, for showing it.
//
// Bounded, because the caller is a page: a browser given a hundred megabytes of
// a log file it asked to preview has been handed a problem rather than a file.
// The truncation is reported so the page can say so rather than quietly
// showing part of something.
func ReadFile(ctx context.Context, accountID, name string, limit int) (text string, truncated bool, err error) {
	path, err := under(accountID, name)
	if err != nil {
		return "", false, err
	}
	if limit <= 0 {
		limit = 64 << 10
	}
	res, err := container.Exec(ctx, container.Run{
		Name:  machineFor(accountID),
		Shell: "bash",
		// head -c, so a single-line minified file is bounded too. A line-based
		// bound is no bound at all on the files this agent writes.
		Command: "head -c " + strconv.Itoa(limit+1) + " -- " + quoted(path),
		Dir:     home(accountID),
		User:    runAs(accountID),
		Wait:    quickWait,
	})
	if err != nil {
		return "", false, err
	}
	out := res.Out
	if len(out) > limit {
		return out[:limit], true, nil
	}
	return out, false, nil
}

// listed reports whether a container of exactly this name is in the list.
//
// The runtime filters by substring, so asking for "mu-shell-ann" also returns
// "mu-shell-anna". Two accounts, one answer, and the one that arrives is
// whichever the daemon lists first.
func listed(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}
