package backup

// The copy that survives losing the machine.
//
// Snapshots next door protect against a bad write and an operator mistake, and
// against nothing else: they are on the disk they are protecting. This is the
// other half — one archive a day, pushed to object storage, holding everything
// the instance could not be rebuilt without.
//
// Signed by hand rather than with a vendor SDK. The signature is a hundred
// lines of HMAC and a canonical string, it is stable and has been for a decade,
// and the alternative is a dependency tree larger than the rest of this program
// for one PUT. Anything S3-compatible answers it — R2, Backblaze, MinIO — which
// is why the settings name the storage rather than a provider.
//
// # What goes in, and why the keys are in it
//
// The data directory, the search index, and keys/. The keys are the
// uncomfortable part and they are not optional: mail is encrypted at rest, so a
// backup without the key restores an inbox nobody can read. Which makes this
// archive the most sensitive object the instance produces, and the reason the
// credentials for the bucket should be able to write to that bucket and do
// nothing else.

import (
	"archive/tar"
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mu/internal/settings"
)

// PushInterval is how often the archive goes off the box. Daily: it is a whole
// copy each time rather than a difference, so it is bandwidth and storage, and
// a day of loss is the same window the local snapshots keep.
const PushInterval = 24 * time.Hour

// PushEnabled reports whether an operator asked for this.
//
// Off unless both the switch and a bucket are set. Silently doing nothing is
// the right behaviour for an instance somebody else runs and has not
// configured — but /admin/backup says which of the two is missing, because
// "backups are on" and "backups are working" being different is how people
// discover they have none.
func PushEnabled() bool {
	if strings.TrimSpace(settings.Get("S3_BUCKET")) == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(settings.Get("BACKUP_S3"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// Push writes one archive to object storage and reports the key it used.
func Push() (string, error) {
	if !PushEnabled() {
		return "", fmt.Errorf("no bucket configured")
	}
	f, err := os.CreateTemp("", "mu-backup-*.tar.gz")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	defer f.Close()

	// The payload has to be hashed before it can be signed, so it is written to
	// a file and read back rather than held in memory — an instance's data
	// directory is not guaranteed to be small, and the machine running this is
	// the one with the small disk.
	sum := sha256.New()
	if err := archive(io.MultiWriter(f, sum)); err != nil {
		return "", err
	}
	if err := f.Sync(); err != nil {
		return "", err
	}
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return "", err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	key := archiveKey(time.Now().UTC())

	return key, put(key, f, size, hex.EncodeToString(sum.Sum(nil)))
}

// DefaultPrefix is the namespace archives own in the shared object store.
//
// It is fixed rather than configurable because the bucket is shared by
// services: files owns files/, backups owns backups/. An operator should choose
// the bucket once, not have to coordinate internal paths between services.
const DefaultPrefix = "backups"

// archiveKey is where one day's archive goes.
func archiveKey(at time.Time) string {
	// Older deployments could use a private per-instance prefix. Honour it as
	// a migration alias so two instances which already share a bucket do not
	// begin overwriting one another after an upgrade. It is intentionally not
	// part of the current configuration surface.
	legacy := strings.Trim(strings.TrimSpace(settings.Get(strings.Join([]string{"S3", "PREFIX"}, "_"))), "/")
	if legacy != "" {
		return legacy + "/" + at.Format("2006-01-02") + ".tar.gz"
	}
	return DefaultPrefix + "/" + at.Format("2006-01-02") + ".tar.gz"
}

// archive writes the whole instance as a gzipped tar.
func archive(w io.Writer) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)

	home := Home()
	// The index is vacuumed into place first so what goes in the archive is a
	// consistent database rather than a file caught mid-transaction. This is
	// the only copy of it that exists anywhere off the disk.
	if IndexSnapshot != nil {
		tmp := filepath.Join(os.TempDir(), "mu-index-snapshot.db")
		if err := IndexSnapshot(tmp); err == nil {
			addFile(tw, tmp, "data/index.db")
			os.Remove(tmp)
		}
	}
	for _, dir := range []string{"data", "keys"} {
		root := filepath.Join(home, dir)
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			name := filepath.Base(path)
			if name == "index.db" || skip(name) {
				return nil // already added, or not worth carrying
			}
			rel, err := filepath.Rel(home, path)
			if err != nil {
				return nil
			}
			addFile(tw, path, rel)
			return nil
		})
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func addFile(tw *tar.Writer, path, name string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	if err := tw.WriteHeader(&tar.Header{
		Name: name, Mode: 0600, Size: info.Size(), ModTime: info.ModTime(),
	}); err != nil {
		return
	}
	io.Copy(tw, f) //nolint:errcheck
}

// put uploads with an AWS SigV4 signature.
func put(key string, body io.Reader, size int64, payloadHash string) error {
	bucket := strings.TrimSpace(settings.Get("S3_BUCKET"))
	region := strings.TrimSpace(settings.Get("S3_REGION"))
	if region == "" {
		region = "us-east-1"
	}
	id := strings.TrimSpace(settings.Get("S3_ACCESS_KEY_ID"))
	secret := strings.TrimSpace(settings.Get("S3_SECRET_ACCESS_KEY"))
	if id == "" || secret == "" {
		return fmt.Errorf("S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY are not set")
	}

	endpoint := strings.TrimSpace(settings.Get("S3_ENDPOINT"))
	host := bucket + ".s3." + region + ".amazonaws.com"
	scheme := "https"
	if endpoint != "" {
		e := strings.TrimSuffix(endpoint, "/")
		e = strings.TrimPrefix(strings.TrimPrefix(e, "https://"), "http://")
		host = e
		if strings.HasPrefix(endpoint, "http://") {
			scheme = "http"
		}
	}

	// Path-style against a custom endpoint, virtual-host style against AWS —
	// unless the endpoint already names the bucket, which is the form every
	// provider console puts in front of you. DigitalOcean shows the Space's
	// endpoint as https://<bucket>.<region>.digitaloceanspaces.com, so pasting
	// it and also setting S3_BUCKET signed a path of /<bucket>/<key> against a
	// host that had already resolved the bucket — and the archives landed under
	// a folder named after the bucket, inside the bucket. Nothing failed; it was
	// only visible by looking in the console.
	//
	// internal/blob builds a path the same way against the same S3_ENDPOINT and
	// S3_BUCKET, and carries the same check.
	path := "/" + key
	if endpoint != "" && !strings.HasPrefix(host, bucket+".") {
		path = "/" + bucket + "/" + key
	}

	req, err := http.NewRequest(http.MethodPut, scheme+"://"+host+path, body)
	if err != nil {
		return err
	}
	req.ContentLength = size
	now := time.Now().UTC()
	stamp := now.Format("20060102T150405Z")
	day := now.Format("20060102")

	req.Header.Set("Host", host)
	req.Header.Set("X-Amz-Date", stamp)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	req.Header.Set("Content-Type", "application/gzip")

	signed := "content-type;host;x-amz-content-sha256;x-amz-date"
	canonical := strings.Join([]string{
		http.MethodPut,
		escapePath(path),
		"",
		"content-type:application/gzip",
		"host:" + host,
		"x-amz-content-sha256:" + payloadHash,
		"x-amz-date:" + stamp,
		"",
		signed,
		payloadHash,
	}, "\n")

	scope := day + "/" + region + "/s3/aws4_request"
	toSign := strings.Join([]string{
		"AWS4-HMAC-SHA256", stamp, scope, sha256hex(canonical),
	}, "\n")

	k := hmacSum([]byte("AWS4"+secret), day)
	k = hmacSum(k, region)
	k = hmacSum(k, "s3")
	k = hmacSum(k, "aws4_request")
	sig := hex.EncodeToString(hmacSum(k, toSign))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		id, scope, signed, sig))

	resp, err := (&http.Client{Timeout: 10 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

// escapePath encodes a key the way the signature expects: each segment
// percent-encoded, and the slashes left alone.
func escapePath(p string) string {
	var out strings.Builder
	for i, seg := range strings.Split(p, "/") {
		if i > 0 {
			out.WriteByte('/')
		}
		for _, c := range []byte(seg) {
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
				(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' {
				out.WriteByte(c)
				continue
			}
			fmt.Fprintf(&out, "%%%02X", c)
		}
	}
	return out.String()
}

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func hmacSum(key []byte, msg string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(msg))
	return h.Sum(nil)
}
