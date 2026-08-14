package wallet

import (
	"testing"

	"mu/internal/data"
)

func writeStore(t *testing.T, name string, m map[string]*BaseWallet) {
	t.Helper()
	if err := data.SaveJSON(name, m); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { data.DeleteFile(name) }) //nolint:errcheck
}
