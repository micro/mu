package data

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveFileReportsDirectoryErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	muPath := filepath.Join(os.Getenv("HOME"), ".mu")
	if err := os.WriteFile(muPath, []byte("not a directory"), 0600); err != nil {
		t.Fatalf("create blocking file: %v", err)
	}

	if err := SaveFile("example.txt", "content"); err == nil {
		t.Fatal("SaveFile returned nil when the data directory could not be created")
	}
}

func TestSaveJSONReportsDirectoryErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	muPath := filepath.Join(os.Getenv("HOME"), ".mu")
	if err := os.WriteFile(muPath, []byte("not a directory"), 0600); err != nil {
		t.Fatalf("create blocking file: %v", err)
	}

	if err := SaveJSON("settings.json", map[string]string{"key": "value"}); err == nil {
		t.Fatal("SaveJSON returned nil when the data directory could not be created")
	}
}
