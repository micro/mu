package blob

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The signing key is four chained HMACs and every input matters: get the order,
// the service string or the "AWS4" prefix wrong and every request 403s with no
// clue which part was wrong.
//
// The expected value is AWS's published example inputs — secret
// wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY, 20130524, us-east-1, s3 — run
// through an independent implementation of the same algorithm, not copied from
// a constant. Two constants were tried from memory while writing this and both
// were wrong; agreement between two implementations written separately is the
// evidence here, and it is worth more than a number nobody can source.
func TestSigningKeyDerivation(t *testing.T) {
	const (
		secret = "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"
		want   = "f117494eff5d09da21cbf7f0339559ea04fc9582d31299cb992be70a6b27c97a"
	)
	if got := hex.EncodeToString(signingKey(secret, "20130524", "us-east-1")); got != want {
		t.Errorf("signing key = %s, want %s", got, want)
	}

	// Every input has to reach the key, or a stale signature would be accepted
	// across days, regions or accounts.
	base := hex.EncodeToString(signingKey(secret, "20130524", "us-east-1"))
	for name, got := range map[string]string{
		"date":   hex.EncodeToString(signingKey(secret, "20130525", "us-east-1")),
		"region": hex.EncodeToString(signingKey(secret, "20130524", "lon1")),
		"secret": hex.EncodeToString(signingKey(secret+"x", "20130524", "us-east-1")),
	} {
		if got == base {
			t.Errorf("changing the %s did not change the signing key", name)
		}
	}
}

// The canonical request is the part that goes wrong silently. This is AWS's
// own example request and its published canonical form.
func TestCanonicalRequestMatchesTheAWSExample(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	const emptyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	req.Header.Set("x-amz-content-sha256", emptyHash)
	req.Header.Set("x-amz-date", "20130524T000000Z")
	req.Host = "examplebucket.s3.amazonaws.com"

	canonical, signed := canonicalRequest(req, emptyHash)

	if signed != "host;x-amz-content-sha256;x-amz-date" {
		t.Errorf("signed headers = %q", signed)
	}
	want := strings.Join([]string{
		"GET",
		"/test.txt",
		"",
		"host:examplebucket.s3.amazonaws.com",
		"x-amz-content-sha256:" + emptyHash,
		"x-amz-date:20130524T000000Z",
		"",
		"host;x-amz-content-sha256;x-amz-date",
		emptyHash,
	}, "\n")
	if canonical != want {
		t.Errorf("canonical request:\n%q\nwant:\n%q", canonical, want)
	}
}

// AWS escapes a space as %20, not +, and leaves a tilde alone. url.QueryEscape
// does neither, which is why this is hand-rolled.
func TestURIEscapeFollowsAWSRules(t *testing.T) {
	for in, want := range map[string]string{
		"simple":       "simple",
		"with space":   "with%20space",
		"tilde~":       "tilde~",
		"plus+sign":    "plus%2Bsign",
		"slash/inside": "slash%2Finside",
		"a-b_c.d":      "a-b_c.d",
	} {
		if got := uriEscape(in); got != want {
			t.Errorf("uriEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

// Key structure survives escaping: the slashes between segments are the shape
// of the key, not characters in it.
func TestEscapeKeyKeepsItsStructure(t *testing.T) {
	if got := escapeKey("files/alice/my file.txt"); got != "files/alice/my%20file.txt" {
		t.Errorf("escapeKey = %q", got)
	}
}

// A round trip against a stub that checks the request looks like S3.
func TestS3RoundTrip(t *testing.T) {
	objects := map[string][]byte{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=") {
			t.Errorf("request was not signed: %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("x-amz-content-sha256") == "" || r.Header.Get("x-amz-date") == "" {
			t.Error("missing required amz headers")
		}
		key := strings.TrimPrefix(r.URL.Path, "/test-bucket/")

		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			// The payload hash must describe the body actually sent.
			sum := sha256.Sum256(body)
			if hex.EncodeToString(sum[:]) != r.Header.Get("x-amz-content-sha256") {
				t.Error("payload hash does not match the body")
			}
			if r.Header.Get("x-amz-acl") != "private" {
				t.Error("object was not stored private")
			}
			objects[key] = body
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			b, ok := objects[key]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Write(b)
		case http.MethodDelete:
			delete(objects, key)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	s := &s3Store{
		endpoint: srv.URL, bucket: "test-bucket", region: "lon1",
		accessKey: "key", secretKey: "secret",
		client: srv.Client(), now: func() time.Time { return time.Unix(0, 0).UTC() },
	}

	content := []byte("date,total\n2026-08-03,41\n")
	if err := s.Put("files/alice/abc", content, "text/csv"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("files/alice/abc")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("contents changed: %q", got)
	}
	if err := s.Delete("files/alice/abc"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("files/alice/abc"); err == nil {
		t.Error("the object survived deletion")
	}
}

// Deleting something already gone is not a failure — the caller asked for it to
// be absent and it is.
func TestDeletingAMissingObjectSucceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s := &s3Store{
		endpoint: srv.URL, bucket: "b", region: "r", accessKey: "k", secretKey: "s",
		client: srv.Client(), now: time.Now,
	}
	if err := s.Delete("nope"); err != nil {
		t.Errorf("deleting a missing object failed: %v", err)
	}
}

// Configuration is all-or-nothing. Half a bucket is a mistake worth reporting
// rather than a store to half-use.
func TestConfigurationIsAllOrNothing(t *testing.T) {
	t.Setenv("S3_ENDPOINT", "")
	t.Setenv("S3_BUCKET", "")
	s, err := newS3FromSettings()
	if s != nil || err != nil {
		t.Errorf("unconfigured should mean no store and no error, got %v, %v", s, err)
	}

	t.Setenv("S3_ENDPOINT", "https://lon1.digitaloceanspaces.com")
	if _, err := newS3FromSettings(); err == nil {
		t.Error("an endpoint with no bucket was accepted")
	}

	t.Setenv("S3_BUCKET", "mu-files")
	if _, err := newS3FromSettings(); err == nil {
		t.Error("a bucket with no credentials was accepted")
	}

	t.Setenv("S3_ACCESS_KEY", "k")
	t.Setenv("S3_SECRET_KEY", "s")
	got, err := newS3FromSettings()
	if err != nil || got == nil {
		t.Fatalf("a complete configuration was refused: %v", err)
	}
	if got.region != "us-east-1" {
		t.Errorf("region should default, got %q", got.region)
	}
}

// A bare hostname is a reasonable thing to paste; it should not produce an
// unusable endpoint.
func TestEndpointGetsAScheme(t *testing.T) {
	t.Setenv("S3_ENDPOINT", "lon1.digitaloceanspaces.com")
	t.Setenv("S3_BUCKET", "mu-files")
	t.Setenv("S3_ACCESS_KEY", "k")
	t.Setenv("S3_SECRET_KEY", "s")

	s, err := newS3FromSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(s.endpoint, "https://") {
		t.Errorf("endpoint = %q, want an https scheme", s.endpoint)
	}
}

// The endpoint a provider console gives you already names the bucket.
//
// DigitalOcean shows a Space as https://<bucket>.<region>.digitaloceanspaces.com.
// Pasting that and also setting S3_BUCKET put the bucket in the path of a host
// that had already resolved it, so every object landed in a folder named after
// the bucket, inside the bucket. Nothing failed — it was only visible by looking
// in the console, which is how it survived.
func TestABucketQualifiedEndpointIsNotRepeatedInThePath(t *testing.T) {
	t.Setenv("S3_ENDPOINT", "https://micro.lon1.digitaloceanspaces.com")
	t.Setenv("S3_BUCKET", "micro")
	t.Setenv("S3_ACCESS_KEY", "k")
	t.Setenv("S3_SECRET_KEY", "s")

	s, err := newS3FromSettings()
	if err != nil {
		t.Fatal(err)
	}
	req, err := s.request("PUT", "files/report.csv", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := req.URL.Path, "/files/report.csv"; got != want {
		t.Errorf("path = %q, want %q — the host already names the bucket", got, want)
	}
}

// The regional endpoint does not, so the path carries it.
func TestARegionalEndpointKeepsTheBucketInThePath(t *testing.T) {
	t.Setenv("S3_ENDPOINT", "https://lon1.digitaloceanspaces.com")
	t.Setenv("S3_BUCKET", "micro")
	t.Setenv("S3_ACCESS_KEY", "k")
	t.Setenv("S3_SECRET_KEY", "s")

	s, err := newS3FromSettings()
	if err != nil {
		t.Fatal(err)
	}
	req, err := s.request("PUT", "files/report.csv", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := req.URL.Path, "/micro/files/report.csv"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}
