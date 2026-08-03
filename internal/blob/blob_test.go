package blob

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-blob-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func unconfigure(t *testing.T) {
	t.Helper()
	for _, k := range []string{"S3_ENDPOINT", "S3_BUCKET", "S3_ACCESS_KEY", "S3_SECRET_KEY", "S3_REGION"} {
		t.Setenv(k, "")
	}
	reset()
}

// reset drops the cached store so a test's settings take effect.
func reset() {
	mu.Lock()
	current, configured = nil, ""
	mu.Unlock()
}

// With nothing configured, this is the disk — a self-hoster who wants one
// binary and a directory keeps having that.
func TestDefaultsToLocalDisk(t *testing.T) {
	unconfigure(t)
	if _, ok := Default().(Local); !ok {
		t.Errorf("unconfigured store is %s, want local disk", Default().Name())
	}

	if err := Put("blobtest/one", []byte("hello"), "text/plain"); err != nil {
		t.Fatal(err)
	}
	got, err := Get("blobtest/one")
	if err != nil || string(got) != "hello" {
		t.Fatalf("local round trip failed: %q %v", got, err)
	}
	if err := Delete("blobtest/one"); err != nil {
		t.Fatal(err)
	}
	if _, err := Get("blobtest/one"); err == nil {
		t.Error("the file survived deletion")
	}
}

// Switching an instance that already has files must not make them disappear.
// Everything written before the switch is on disk; new writes go to the bucket;
// reads have to find both, or turning this on looks like data loss.
func TestReadsFallBackToDiskAfterSwitching(t *testing.T) {
	unconfigure(t)

	// A file written before object storage was configured.
	if err := Put("blobtest/old", []byte("written before the switch"), "text/plain"); err != nil {
		t.Fatal(err)
	}

	// Now switch to a bucket that has never heard of it.
	objects := map[string][]byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/b/")
		switch r.Method {
		case http.MethodPut:
			buf := make([]byte, r.ContentLength)
			r.Body.Read(buf)
			objects[key] = buf
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			b, ok := objects[key]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Write(b)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	t.Setenv("S3_ENDPOINT", srv.URL)
	t.Setenv("S3_BUCKET", "b")
	t.Setenv("S3_ACCESS_KEY", "k")
	t.Setenv("S3_SECRET_KEY", "s")
	reset()
	defer reset()

	if _, ok := Default().(Local); ok {
		t.Fatal("configuring a bucket did not switch the store")
	}

	got, err := Get("blobtest/old")
	if err != nil {
		t.Fatalf("a file written before the switch became unreadable: %v", err)
	}
	if string(got) != "written before the switch" {
		t.Errorf("wrong contents: %q", got)
	}
}

// A half-configured bucket is a mistake worth seeing, but it must not take file
// storage down with it.
func TestBrokenConfigurationFallsBackRatherThanFailing(t *testing.T) {
	unconfigure(t)
	t.Setenv("S3_ENDPOINT", "https://lon1.digitaloceanspaces.com")
	t.Setenv("S3_BUCKET", "") // half-configured
	reset()
	defer reset()

	if _, ok := Default().(Local); !ok {
		t.Errorf("a broken configuration produced %s instead of falling back to disk", Default().Name())
	}
}

// Changing the setting at /admin/env takes effect without a restart.
func TestConfigurationChangeIsPickedUp(t *testing.T) {
	unconfigure(t)
	if _, ok := Default().(Local); !ok {
		t.Fatal("expected local to start")
	}

	t.Setenv("S3_ENDPOINT", "https://example.invalid")
	t.Setenv("S3_BUCKET", "b")
	t.Setenv("S3_ACCESS_KEY", "k")
	t.Setenv("S3_SECRET_KEY", "s")
	defer reset()

	if _, ok := Default().(Local); ok {
		t.Error("the store did not change when the settings did")
	}
}
