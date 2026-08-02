package files

import (
	"context"
	"encoding/base64"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"mu/internal/data"
	"mu/internal/service"
)

// Storage is rooted at $HOME/.mu/data, so each run gets its own HOME rather
// than writing into a developer's real one.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-files-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// A file round-trips: what goes in comes back out, byte for byte.
func TestPutAndGetRoundTrip(t *testing.T) {
	f, err := Put("alice", "report.csv", "", "date,total\n2026-08-02,41\n", "")
	if err != nil {
		t.Fatal(err)
	}
	if f.Type != "text/csv; charset=utf-8" && !strings.HasPrefix(f.Type, "text/csv") {
		t.Errorf("content type was not guessed from the name: %q", f.Type)
	}
	if f.URL == "" || !strings.HasSuffix(f.URL, f.ID) {
		t.Errorf("no usable URL: %q", f.URL)
	}

	got, raw, err := Get("alice", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "date,total\n2026-08-02,41\n" {
		t.Errorf("contents changed in storage: %q", raw)
	}
	if got.Checksum != f.Checksum {
		t.Error("checksum changed between write and read")
	}
	_ = Delete("alice", f.ID)
}

// Binary survives too, which is the point of accepting base64 at all.
func TestBinaryRoundTrip(t *testing.T) {
	raw := []byte{0x00, 0x01, 0xff, 0xfe, 0x7f}
	f, err := Put("alice", "blob.bin", "", base64.StdEncoding.EncodeToString(raw), "base64")
	if err != nil {
		t.Fatal(err)
	}
	_, got, err := Get("alice", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Errorf("binary contents changed: %v", got)
	}
	_ = Delete("alice", f.ID)
}

// One account cannot read another's private file. This is the whole security
// model of the service in one assertion.
func TestPrivateFilesAreNotReadableByOthers(t *testing.T) {
	f, err := Put("alice", "secret.txt", "", "for alice only", "")
	if err != nil {
		t.Fatal(err)
	}
	defer Delete("alice", f.ID)

	if _, _, err := Get("bob", f.ID); err == nil {
		t.Error("bob read alice's private file")
	}
	if _, _, err := Get("", f.ID); err == nil {
		t.Error("a signed-out caller read a private file")
	}
}

// Sharing is what makes the URL worth handing over.
func TestSharedFilesAreReadableByAnyone(t *testing.T) {
	f, err := Put("alice", "public.txt", "", "anyone may read this", "")
	if err != nil {
		t.Fatal(err)
	}
	defer Delete("alice", f.ID)

	if _, err := Share("alice", f.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Get("bob", f.ID); err != nil {
		t.Errorf("a shared file was not readable by another account: %v", err)
	}

	// And it can be taken back.
	if _, err := Share("alice", f.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Get("bob", f.ID); err == nil {
		t.Error("unsharing did not make the file private again")
	}
}

// A name is a name, not a path. "../../etc/passwd" must not mean anything.
func TestNamesCannotEscapeTheDataDirectory(t *testing.T) {
	f, err := Put("alice", "../../etc/passwd", "", "nope", "")
	if err != nil {
		t.Fatal(err)
	}
	defer Delete("alice", f.ID)

	if strings.Contains(f.Name, "/") || strings.Contains(f.Name, "..") {
		t.Errorf("a path survived as a file name: %q", f.Name)
	}
	if key := blobKey("alice", f.ID); strings.Contains(key, "..") {
		t.Errorf("the storage key can climb out of the data directory: %q", key)
	}
}

func TestOversizeFilesAreRefused(t *testing.T) {
	if _, err := Put("alice", "huge.txt", "", strings.Repeat("x", MaxBytes+1), ""); err == nil {
		t.Error("a file over the size limit was accepted")
	}
}

func TestEmptyAndUnnamedFilesAreRefused(t *testing.T) {
	if _, err := Put("alice", "empty.txt", "", "", ""); err == nil {
		t.Error("an empty file was accepted")
	}
	if _, err := Put("alice", "", "", "content", ""); err == nil {
		t.Error("a file with no name was accepted")
	}
	if _, err := Put("", "x.txt", "", "content", ""); err == nil {
		t.Error("a signed-out caller stored a file")
	}
}

func TestBadBase64IsRefused(t *testing.T) {
	if _, err := Put("alice", "x.bin", "", "!!!not base64!!!", "base64"); err == nil {
		t.Error("invalid base64 was accepted")
	}
}

// Deleting removes the bytes, not just the record — otherwise storage fills up
// with files nobody can see or reclaim.
func TestDeleteRemovesTheBytes(t *testing.T) {
	f, err := Put("alice", "gone.txt", "", "temporary", "")
	if err != nil {
		t.Fatal(err)
	}
	key := blobKey("alice", f.ID)
	if _, err := data.LoadFile(key); err != nil {
		t.Fatalf("the file was never written: %v", err)
	}
	if err := Delete("alice", f.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := data.LoadFile(key); err == nil {
		t.Error("the bytes are still on disk after delete")
	}
}

// The handler must not serve caller-supplied bytes inline: an HTML or SVG
// upload rendered on this origin would run script as Mu.
func TestServedFilesAreDownloadsNotPages(t *testing.T) {
	f, err := Put("alice", "evil.html", "", "<script>alert(1)</script>", "")
	if err != nil {
		t.Fatal(err)
	}
	defer Delete("alice", f.ID)
	if _, err := Share("alice", f.ID, true); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	Handler(rec, httptest.NewRequest("GET", "/files/"+f.ID, nil))

	if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") {
		t.Errorf("a stored file is served inline (Content-Disposition %q)", cd)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("the browser is allowed to sniff a stored file's type")
	}
}

// A file name goes into a header, where a quote would let a caller write
// headers of their own.
func TestFileNamesCannotForgeHeaders(t *testing.T) {
	got := quoteName("in\"jected\r\nX-Evil: yes")
	if strings.ContainsAny(got[1:len(got)-1], "\"\r\n") {
		t.Errorf("a file name can break out of the header: %q", got)
	}
}

// The service handler binds the owner from the call context, never a field.
func TestServiceBindsOwnerFromContext(t *testing.T) {
	var put PutResponse
	err := Server{}.Put(service.WithAccount(context.Background(), "carol"),
		&PutRequest{Name: "ctx.txt", Content: "hello"}, &put)
	if err != nil {
		t.Fatal(err)
	}
	if put.File.Owner != "carol" {
		t.Errorf("owner is %q, want carol", put.File.Owner)
	}
	defer Delete("carol", put.File.ID)

	// And a caller with no account cannot store at all.
	if err := (Server{}).Put(context.Background(), &PutRequest{Name: "x.txt", Content: "x"}, &PutResponse{}); err == nil {
		t.Error("a caller with no account stored a file")
	}
}

func TestTextComesBackAsTextAndBinaryAsBase64(t *testing.T) {
	if s, binary := encodeForWire("text/csv", []byte("a,b\n1,2")); binary || s != "a,b\n1,2" {
		t.Errorf("text came back encoded: %q binary=%v", s, binary)
	}
	if s, binary := encodeForWire("application/pdf", []byte{0x00, 0xff}); !binary || s == "" {
		t.Errorf("binary did not come back as base64: %q binary=%v", s, binary)
	}
	// A file labelled text but holding invalid UTF-8 must not be pasted into
	// JSON as if it were a string.
	if _, binary := encodeForWire("text/plain", []byte{0xff, 0xfe}); !binary {
		t.Error("invalid UTF-8 was returned as text")
	}
}
