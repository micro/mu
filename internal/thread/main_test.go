package thread

import (
	"os"
	"testing"
)

// The record's own tests do not run against the machine's record.
//
// Most of them never set HOME, and for as long as the store was read in init()
// that made no difference either way: HOME set inside a test came too late, so
// every test in this package worked on the real ~/.mu whether it asked to or
// not. It read a person's conversations and the flusher wrote its fixtures back
// a second later. The store on the machine where this was found held 38
// threads, 37 of them named bridge_reader, mailbox_unread, act_forged and the
// like.
//
// One line here rather than one line per test file, because the guarantee
// wanted is "this package never touches the real record", and a rule that has
// to be remembered in each new test file is a rule that will be missed. Tests
// that want their own record still set HOME themselves; they get a fresh one,
// because the record follows HOME now.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "thread-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
