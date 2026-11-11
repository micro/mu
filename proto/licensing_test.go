// SPDX-License-Identifier: Apache-2.0

package proto_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContractLicenceNotices(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".proto") && !strings.HasSuffix(path, ".go") {
			return nil
		}
		want := "// SPDX-License-Identifier: Apache-2.0"
		if strings.HasPrefix(filepath.ToSlash(path), "generate/") {
			want = "// SPDX-License-Identifier: AGPL-3.0-only"
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(string(raw), want+"\n") {
			t.Errorf("%s: missing %s", path, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Keep the generator's explicit exception aligned with the repository default.
	root, err := os.ReadFile("../LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	generator, err := os.ReadFile("generate/LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	if string(root) != string(generator) {
		t.Fatal("generator licence differs from root AGPL")
	}
}
