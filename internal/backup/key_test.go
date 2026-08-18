package backup

import (
	"strings"
	"testing"
	"time"
)

func at(t *testing.T) time.Time {
	t.Helper()
	when, err := time.Parse("2006-01-02", "2026-08-18")
	if err != nil {
		t.Fatal(err)
	}
	return when
}

// The bucket is S3_BUCKET. The prefix names a directory inside it.
//
// Setting the bucket to "micro" and the prefix to "micro/mu" — or, as happened,
// pasting a bucket-qualified endpoint — produces micro/micro/mu, a folder named
// after the bucket inside the bucket. The prefix is one thing and it is not the
// bucket.
func TestThePrefixIsADirectoryNotTheBucket(t *testing.T) {
	t.Setenv("S3_PREFIX", "backups")
	if got, want := archiveKey(at(t)), "backups/2026-08-18.tar.gz"; got != want {
		t.Errorf("archiveKey = %q, want %q", got, want)
	}

	// Slashes either side are the same thing to a person and different things
	// to S3, so they come off.
	for _, spelling := range []string{"/backups", "backups/", "/backups/"} {
		t.Setenv("S3_PREFIX", spelling)
		if got, want := archiveKey(at(t)), "backups/2026-08-18.tar.gz"; got != want {
			t.Errorf("prefix %q gave %q, want %q", spelling, got, want)
		}
	}
}

// With nothing set, archives go under backups/ rather than the bucket root.
//
// internal/blob reads the same S3_BUCKET for user file storage, so the root is
// not ours: an archive written there sits among the files it is protecting.
func TestArchivesDoNotLandAtTheBucketRoot(t *testing.T) {
	t.Setenv("S3_PREFIX", "")

	key := archiveKey(at(t))
	if !strings.HasPrefix(key, DefaultPrefix+"/") {
		t.Fatalf("archiveKey = %q, want it under %q", key, DefaultPrefix)
	}
	if strings.Count(key, "/") != 1 {
		t.Errorf("archiveKey = %q, want one directory deep", key)
	}
}
