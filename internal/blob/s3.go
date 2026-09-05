package blob

// An S3-compatible store, signed by hand.
//
// The AWS SDK would work and is enormous — hundreds of packages for four
// requests. This codebase already implements secp256k1, RLP and ECDSA rather
// than take a dependency for them (service/wallet/evm.go), and SigV4 is a
// smaller job than any of those: a canonical request, a string to sign, four
// HMACs. That is the whole protocol below.
//
// Written against the S3 REST API rather than any provider's SDK, so the same
// code reaches DigitalOcean Spaces, Cloudflare R2, Backblaze B2, MinIO and S3.

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"mu/internal/settings"
)

type s3Store struct {
	endpoint  string // https://lon1.digitaloceanspaces.com
	bucket    string
	region    string
	accessKey string
	secretKey string
	client    *http.Client
	// now is overridable so signing can be tested against a fixed timestamp.
	now func() time.Time
}

// newS3FromSettings builds the store from configuration, or returns nil when
// object storage is not configured at all.
func newS3FromSettings() (*s3Store, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(settings.Get("S3_ENDPOINT")), "/")
	bucket := strings.TrimSpace(settings.Get("S3_BUCKET"))
	if endpoint == "" && bucket == "" {
		return nil, nil // not configured; the local disk is the answer
	}
	if bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET is required when S3_ENDPOINT is set")
	}

	access := s3AccessKey()
	secret := s3SecretKey()
	if access == "" || secret == "" {
		return nil, fmt.Errorf("S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY are required")
	}

	region := strings.TrimSpace(settings.Get("S3_REGION"))
	if region == "" {
		// Providers that ignore the region still require one in the signature.
		region = "us-east-1"
	}
	if endpoint == "" {
		endpoint = "https://s3." + region + ".amazonaws.com"
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}

	return &s3Store{
		endpoint: endpoint, bucket: bucket, region: region,
		accessKey: access, secretKey: secret,
		client: httpClient(),
		now:    time.Now,
	}, nil
}

// s3AccessKey and s3SecretKey keep one canonical pair for object storage while
// still accepting the names used by older Files deployments. The old names are
// deliberately assembled here rather than advertised as current settings: they
// are a migration path, not a second configuration surface.
func s3AccessKey() string {
	if v := strings.TrimSpace(settings.Get("S3_ACCESS_KEY_ID")); v != "" {
		return v
	}
	return strings.TrimSpace(settings.Get(strings.Join([]string{"S3", "ACCESS", "KEY"}, "_")))
}

func s3SecretKey() string {
	if v := strings.TrimSpace(settings.Get("S3_SECRET_ACCESS_KEY")); v != "" {
		return v
	}
	return strings.TrimSpace(settings.Get(strings.Join([]string{"S3", "SECRET", "KEY"}, "_")))
}

// httpClient is the client S3 requests use, shared so a test can build a store
// without duplicating the timeout.
func httpClient() *http.Client { return &http.Client{Timeout: 60 * time.Second} }

func (s *s3Store) Name() string { return "S3 (" + s.bucket + ")" }

func (s *s3Store) Put(key string, content []byte, contentType string) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	req, err := s.request(http.MethodPut, key, content)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	// Objects here are served through Mu, which checks who is asking. The
	// bucket itself must never be public: a private file readable by anyone
	// holding the object URL would route around that check entirely.
	req.Header.Set("x-amz-acl", "private")
	s.sign(req, content)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return s3Error("put", key, resp)
	}
	return nil
}

func (s *s3Store) Get(key string) ([]byte, error) {
	req, err := s.request(http.MethodGet, key, nil)
	if err != nil {
		return nil, err
	}
	s.sign(req, nil)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, s3Error("get", key, resp)
	}
	return io.ReadAll(resp.Body)
}

func (s *s3Store) Delete(key string) error {
	req, err := s.request(http.MethodDelete, key, nil)
	if err != nil {
		return err
	}
	s.sign(req, nil)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// S3 answers 204 for a delete whether or not the object was there, which is
	// the behaviour we want: deleting something already gone is not a failure.
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		return s3Error("delete", key, resp)
	}
	return nil
}

func (s *s3Store) request(method, key string, body []byte) (*http.Request, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	// Path-style, unless the endpoint already names the bucket.
	//
	// Every provider console hands you the bucket-qualified endpoint —
	// DigitalOcean shows a Space as https://<bucket>.<region>.digitaloceanspaces.com
	// — so pasting that and also setting S3_BUCKET puts the bucket in the path
	// of a host that has already resolved it, and every object lands in a folder
	// named after the bucket, inside the bucket. It does not fail; it is only
	// visible by looking. internal/backup carries the same check.
	escaped := "/" + escapeKey(key)
	if !s.endpointNamesBucket() {
		escaped = "/" + s.bucket + escaped
	}
	req, err := http.NewRequest(method, s.endpoint+escaped, r)
	if err != nil {
		return nil, err
	}
	// Pin the escaped form.
	//
	// Go only keeps RawPath when it differs from what it would produce itself,
	// and it considers "%20" the canonical encoding of a space — so for a key
	// like "my report.csv" it drops RawPath, EscapedPath falls back to escaping
	// Path, and the two can disagree with our own escaping. Signing a path that
	// is not byte-for-byte what goes on the wire is a 403 saying only
	// "SignatureDoesNotMatch", which is what a real server returned for exactly
	// this key while a stub happily accepted it.
	//
	// AWS also wants characters escaped that RFC 3986 permits raw in a path —
	// "+" among them — so the escaping has to be ours either way.
	req.URL.RawPath = escaped
	return req, nil
}

// endpointNamesBucket reports whether the endpoint host is already the bucket's
// own — the virtual-host form, where the path must not repeat it.
func (s *s3Store) endpointNamesBucket() bool {
	host := strings.TrimPrefix(strings.TrimPrefix(s.endpoint, "https://"), "http://")
	return s.bucket != "" && strings.HasPrefix(host, s.bucket+".")
}

func s3Error(op, key string, resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return fmt.Errorf("s3 %s %q: %s: %s", op, key, resp.Status, strings.TrimSpace(string(b)))
}

// escapeKey percent-encodes a key path segment by segment. The slashes between
// segments are structure and must survive; everything else is escaped the way
// the signature will expect.
func escapeKey(key string) string {
	parts := strings.Split(strings.TrimPrefix(key, "/"), "/")
	for i, p := range parts {
		parts[i] = uriEscape(p)
	}
	return strings.Join(parts, "/")
}

// uriEscape is AWS's rule, which is not url.QueryEscape: a space is %20 rather
// than +, and tilde is not escaped.
func uriEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// sign adds the SigV4 Authorization header.
//
// The steps are AWS's, in order: hash the payload, build a canonical request,
// wrap it in a string to sign, derive a signing key by four chained HMACs, and
// sign. Getting any byte of the canonical request wrong produces a 403 with no
// hint as to which byte, which is why signCanonical is separated out and tested
// against AWS's own published example.
func (s *s3Store) sign(req *http.Request, payload []byte) {
	now := s.now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	payloadHash := sha256Hex(payload)
	req.Header.Set("x-amz-content-sha256", payloadHash)
	req.Header.Set("x-amz-date", amzDate)
	if req.Host == "" {
		req.Host = req.URL.Host
	}

	canonical, signedHeaders := canonicalRequest(req, payloadHash)
	scope := strings.Join([]string{dateStamp, s.region, "s3", "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hex.EncodeToString(sha256Sum([]byte(canonical))),
	}, "\n")

	signature := hex.EncodeToString(hmacSHA256(signingKey(s.secretKey, dateStamp, s.region), []byte(stringToSign)))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.accessKey, scope, signedHeaders, signature))
}

// canonicalRequest builds the canonical form AWS signs, and the list of headers
// it covers.
func canonicalRequest(req *http.Request, payloadHash string) (string, string) {
	// Every x-amz- header must be signed, plus host and content-type when set.
	names := []string{"host"}
	values := map[string]string{"host": req.Host}
	for k, v := range req.Header {
		lower := strings.ToLower(k)
		if lower == "content-type" || strings.HasPrefix(lower, "x-amz-") {
			names = append(names, lower)
			values[lower] = strings.TrimSpace(strings.Join(v, ","))
		}
	}
	sort.Strings(names)

	var headers strings.Builder
	for _, n := range names {
		headers.WriteString(n + ":" + values[n] + "\n")
	}
	signed := strings.Join(names, ";")

	query := canonicalQuery(req.URL)
	canonical := strings.Join([]string{
		req.Method,
		canonicalPath(req.URL),
		query,
		headers.String(),
		signed,
		payloadHash,
	}, "\n")
	return canonical, signed
}

// canonicalPath is the path exactly as it goes on the wire.
//
// EscapedPath returns RawPath when it is a valid encoding of Path, which
// request() guarantees by setting it, and falls back to Go's own escaping
// otherwise. Reading RawPath directly would return "" for a path Go decided
// needed no raw form, and sign the decoded path instead.
func canonicalPath(u *url.URL) string {
	if p := u.EscapedPath(); p != "" {
		return p
	}
	return "/"
}

func canonicalQuery(u *url.URL) string {
	q := u.Query()
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vals := q[k]
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, uriEscape(k)+"="+uriEscape(v))
		}
	}
	return strings.Join(parts, "&")
}

func signingKey(secret, dateStamp, region string) []byte {
	k := hmacSHA256([]byte("AWS4"+secret), []byte(dateStamp))
	k = hmacSHA256(k, []byte(region))
	k = hmacSHA256(k, []byte("s3"))
	return hmacSHA256(k, []byte("aws4_request"))
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Sum(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

func sha256Hex(b []byte) string {
	if b == nil {
		b = []byte{}
	}
	return hex.EncodeToString(sha256Sum(b))
}
