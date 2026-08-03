package blob

// A live check against a real S3 server, not a stub. The stub cannot tell us
// whether the signature is actually acceptable — only a real implementation of
// the protocol can, and a wrong signature is a 403 with no explanation.

import (
	"os"
	"testing"
	"time"
)

func TestAgainstRealS3(t *testing.T) {
	endpoint := os.Getenv("MU_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set MU_TEST_S3_ENDPOINT to run against a real server")
	}
	s := &s3Store{
		endpoint:  endpoint,
		bucket:    os.Getenv("MU_TEST_S3_BUCKET"),
		region:    "us-east-1",
		accessKey: os.Getenv("MU_TEST_S3_KEY"),
		secretKey: os.Getenv("MU_TEST_S3_SECRET"),
		client:    httpClient(),
		now:       time.Now,
	}
	content := []byte("live round trip\n")
	if err := s.Put("files/alice/live-test", content, "text/plain"); err != nil {
		t.Fatalf("PUT rejected by a real server: %v", err)
	}
	got, err := s.Get("files/alice/live-test")
	if err != nil {
		t.Fatalf("GET rejected: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("contents changed: %q", got)
	}
	// A key with a space, which is where escaping usually goes wrong.
	if err := s.Put("files/alice/my report.csv", []byte("a,b\n"), "text/csv"); err != nil {
		t.Fatalf("PUT with a space in the key: %v", err)
	}
	if _, err := s.Get("files/alice/my report.csv"); err != nil {
		t.Fatalf("GET with a space in the key: %v", err)
	}
	if err := s.Delete("files/alice/live-test"); err != nil {
		t.Fatalf("DELETE rejected: %v", err)
	}
	t.Log("real S3 server accepted PUT, GET (incl. spaces) and DELETE")
}
