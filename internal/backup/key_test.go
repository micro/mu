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

func TestArchivesUseTheBackupNamespace(t *testing.T) {
	t.Setenv("S3_PREFIX", "")
	if got, want := archiveKey(at(t)), "backups/2026-08-18.tar.gz"; got != want {
		t.Errorf("archiveKey = %q, want %q", got, want)
	}
}

// The shared bucket root belongs to no service. Files use files/ and backups
// use backups/, so adding more durable services does not make their objects
// collide or require another operator setting.
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

func TestExistingInstancePrefixIsPreserved(t *testing.T) {
	t.Setenv("S3_PREFIX", "instances/london")
	if got, want := archiveKey(at(t)), "instances/london/2026-08-18.tar.gz"; got != want {
		t.Errorf("archiveKey = %q, want %q", got, want)
	}
}
