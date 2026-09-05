package files

import (
	"errors"
	"strings"
	"testing"
)

func TestFailedSFTPWriteDoesNotCommitItsPartialPrefix(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	f, err := Put("failed-write", "report.txt", "text/plain", "original", "")
	if err != nil {
		t.Fatal(err)
	}
	fs := &sftpFiles{account: "failed-write"}
	w := &sftpWriter{fs: fs, account: fs.account, id: f.ID, name: f.Name}
	if _, err := w.WriteAt([]byte("partial"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteAt([]byte("too far"), MaxBytes); err == nil {
		t.Fatal("oversized write succeeded")
	}
	if err := w.Close(); err == nil {
		t.Fatal("closing a failed upload succeeded")
	}
	_, raw, err := Get(fs.account, f.ID)
	if err != nil || string(raw) != "original" {
		t.Fatalf("stored file = %q, %v; want original", raw, err)
	}
}

func TestOpenSFTPHandlesShareOneMemoryLimit(t *testing.T) {
	fs := &sftpFiles{account: "bounded"}
	first := &sftpWriter{fs: fs, account: fs.account, name: "first"}
	if _, err := first.WriteAt([]byte("x"), MaxBytes-1); err != nil {
		t.Fatal(err)
	}
	second := &sftpWriter{fs: fs, account: fs.account, name: "second"}
	if _, err := second.WriteAt([]byte("x"), 0); err == nil || !strings.Contains(err.Error(), "session limit") {
		t.Fatalf("second handle error = %v, want session limit", err)
	}
	if err := second.Close(); err == nil {
		t.Fatal("failed second handle committed")
	}
	// Avoid persisting the deliberately sparse maximum-sized first handle.
	first.failed = errors.New("test cleanup")
	if err := first.Close(); err == nil {
		t.Fatal("failed first handle closed successfully")
	}
	if fs.buffered != 0 {
		t.Fatalf("buffered = %d after close, want zero", fs.buffered)
	}
}

func TestSFTPCanCreateAndTruncateToAnEmptyFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	f, err := Replace("empty", "", "empty.txt", "", nil)
	if err != nil || f.Size != 0 {
		t.Fatalf("create empty file = %#v, %v", f, err)
	}
	if _, err := Replace("empty", f.ID, f.Name, "", []byte("not empty")); err != nil {
		t.Fatal(err)
	}
	f, err = Replace("empty", f.ID, f.Name, "", nil)
	if err != nil || f.Size != 0 {
		t.Fatalf("truncate to empty = %#v, %v", f, err)
	}
	_, raw, err := Get("empty", f.ID)
	if err != nil || len(raw) != 0 {
		t.Fatalf("empty contents = %q, %v", raw, err)
	}
}
