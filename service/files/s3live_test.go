package files

// The whole path, against a real object store: files.Put -> internal/blob ->
// an S3 server that verifies signatures. Skipped unless MU_TEST_S3_ENDPOINT is
// set, so an ordinary run needs no infrastructure.
//
// A stub cannot stand in for this. The signature is only meaningfully checked
// by something that implements the protocol, and the first real server this was
// pointed at rejected any key containing a space.

import (
	"os"
	"strings"
	"testing"

	"mu/internal/blob"
)

func TestFilesLandInTheBucket(t *testing.T) {
	if os.Getenv("MU_TEST_S3_ENDPOINT") == "" {
		t.Skip("set MU_TEST_S3_ENDPOINT to run against a real object store")
	}
	for _, kv := range [][2]string{
		{"S3_ENDPOINT", os.Getenv("MU_TEST_S3_ENDPOINT")},
		{"S3_BUCKET", os.Getenv("MU_TEST_S3_BUCKET")},
		{"S3_ACCESS_KEY_ID", os.Getenv("MU_TEST_S3_KEY")},
		{"S3_SECRET_ACCESS_KEY", os.Getenv("MU_TEST_S3_SECRET")},
	} {
		t.Setenv(kv[0], kv[1])
	}

	content := "date,total\n2026-08-03,41\n"
	f, err := Put("alice", "quarterly report.csv", "", content, "")
	if err != nil {
		t.Fatalf("storing a file with object storage configured: %v", err)
	}
	defer Delete("alice", f.ID)

	// It really is in the bucket, not on the disk.
	stored, err := blob.Default().Get(blobKey("alice", f.ID))
	if err != nil {
		t.Fatalf("the file is not in the object store: %v", err)
	}
	if string(stored) != content {
		t.Errorf("contents changed: %q", stored)
	}
	if name := blob.Default().Name(); !strings.HasPrefix(name, "S3") {
		t.Errorf("store is %q, expected the bucket", name)
	}

	// And it reads back through the service.
	_, raw, err := Get("alice", f.ID)
	if err != nil || string(raw) != content {
		t.Errorf("reading back through the service: %q %v", raw, err)
	}
}
