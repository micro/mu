package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func resetForTest(t *testing.T) {
	t.Helper()
	mu.Lock()
	values = map[string]string{}
	mu.Unlock()
	t.Setenv("HOME", t.TempDir())
}

func TestSetGetSourceAndIsSet(t *testing.T) {
	resetForTest(t)

	if got := Get("MU_TEST_SETTING"); got != "" {
		t.Fatalf("Get before Set = %q, want empty", got)
	}
	if IsSet("MU_TEST_SETTING") {
		t.Fatal("IsSet before Set = true, want false")
	}
	if got := Source("MU_TEST_SETTING"); got != "" {
		t.Fatalf("Source before Set = %q, want empty", got)
	}

	Set("MU_TEST_SETTING", "saved-value")

	if got := Get("MU_TEST_SETTING"); got != "saved-value" {
		t.Fatalf("Get after Set = %q, want saved-value", got)
	}
	if !IsSet("MU_TEST_SETTING") {
		t.Fatal("IsSet after Set = false, want true")
	}
	if got := Source("MU_TEST_SETTING"); got != "saved" {
		t.Fatalf("Source after Set = %q, want saved", got)
	}

	t.Setenv("MU_TEST_SETTING", "env-value")
	if got := Get("MU_TEST_SETTING"); got != "env-value" {
		t.Fatalf("Get with env override = %q, want env-value", got)
	}
	if got := Source("MU_TEST_SETTING"); got != "env" {
		t.Fatalf("Source with env override = %q, want env", got)
	}
}

func TestSetEmptyDeletesSavedValue(t *testing.T) {
	resetForTest(t)

	Set("MU_TEST_SETTING", "saved-value")
	Set("MU_TEST_SETTING", "")

	if got := Get("MU_TEST_SETTING"); got != "" {
		t.Fatalf("Get after deleting setting = %q, want empty", got)
	}
	if got := Source("MU_TEST_SETTING"); got != "" {
		t.Fatalf("Source after deleting setting = %q, want empty", got)
	}
}

func TestLoadReadsPersistedSettings(t *testing.T) {
	resetForTest(t)

	Set("MU_TEST_SETTING", "saved-value")
	mu.Lock()
	values = map[string]string{}
	mu.Unlock()

	Load()

	if got := Get("MU_TEST_SETTING"); got != "saved-value" {
		t.Fatalf("Get after Load = %q, want saved-value", got)
	}
}

func TestAllReturnsCopyOfSavedSettings(t *testing.T) {
	resetForTest(t)

	Set("MU_TEST_SETTING", "saved-value")
	all := All()
	all["MU_TEST_SETTING"] = "mutated"

	if got := Get("MU_TEST_SETTING"); got != "saved-value" {
		t.Fatalf("Get after mutating All result = %q, want saved-value", got)
	}
}

func TestSetPersistsSettingsUnderHome(t *testing.T) {
	resetForTest(t)
	home := os.Getenv("HOME")

	Set("MU_TEST_SETTING", "saved-value")

	path := filepath.Join(home, ".mu", "data", "settings.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected settings file at %s: %v", path, err)
	}
}
