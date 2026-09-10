package test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCIBinary(t *testing.T) {
	for _, scenario := range []string{"success", "corrupt", "cannot-run", "restart-fails"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			stage, bin, mocks := filepath.Join(root, "stage"), filepath.Join(root, "bin"), filepath.Join(root, "mocks")
			for _, dir := range []string{stage, bin, mocks} {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			write := func(path, body string) {
				t.Helper()
				if err := os.WriteFile(path, []byte(body), 0755); err != nil {
					t.Fatal(err)
				}
			}
			installed := filepath.Join(bin, "mu")
			old := "#!/bin/sh\necho old\n"
			fresh := "#!/bin/sh\necho new\n"
			if scenario == "cannot-run" {
				fresh = "#!/bin/sh\nexit 1\n"
			}
			write(installed, old)
			write(filepath.Join(stage, "mu"), fresh)
			sum := fmt.Sprintf("%x  mu\n", sha256.Sum256([]byte(fresh)))
			write(filepath.Join(stage, "SHA256SUMS"), sum)
			if scenario == "corrupt" {
				write(filepath.Join(stage, "mu"), fresh+"# truncated transfer\n")
			}
			write(filepath.Join(stage, "enable-zero-downtime.sh"), "#!/bin/sh\nexit 0\n")
			write(filepath.Join(mocks, "systemctl"), `#!/bin/sh
case "$1" in
 show) printf '{ path=%s ; argv[]=%s --serve ; }\n' "$TEST_BINARY" "$TEST_BINARY" ;;
 restart)
  echo restart >> "$TEST_LOG"
  if [ "$TEST_SCENARIO" = restart-fails ] && ! test -e "$TEST_LOG.failed"; then
   touch "$TEST_LOG.failed"
   exit 1
  fi ;;
 is-active) exit 0 ;;
 *) exit 99 ;;
esac
`)
			write(filepath.Join(mocks, "sudo"), "#!/bin/sh\n[ \"$1\" = -n ] && shift\nexec \"$@\"\n")
			// Keep an open handle to verify atomic replacement preserves the old inode.
			handle, err := os.Open(installed)
			if err != nil {
				t.Fatal(err)
			}
			defer handle.Close()
			cmd := exec.Command("sh", at("scripts/deploy/install-binary.sh"), stage)
			log := filepath.Join(root, "restarts")
			cmd.Env = append(os.Environ(), "PATH="+mocks+":"+os.Getenv("PATH"), "TEST_BINARY="+installed, "TEST_SCENARIO="+scenario, "TEST_LOG="+log)
			out, err := cmd.CombinedOutput()
			if (err == nil) != (scenario == "success") {
				t.Fatalf("unexpected result %v: %s", err, out)
			}
			got, err := os.ReadFile(installed)
			if err != nil {
				t.Fatal(err)
			}
			want := old
			if scenario == "success" {
				want = fresh
			}
			if string(got) != want {
				t.Fatalf("installed binary = %q, want %q", got, want)
			}
			before := make([]byte, len(old))
			if _, err := handle.ReadAt(before, 0); err != nil {
				t.Fatal(err)
			}
			if string(before) != old {
				t.Fatal("running binary was overwritten in place")
			}
			restarts, _ := os.ReadFile(log)
			count := strings.Count(string(restarts), "restart")
			expected := 0
			if scenario == "success" {
				expected = 1
			}
			if scenario == "restart-fails" {
				expected = 2
			}
			if count != expected {
				t.Fatalf("restart count %d, want %d", count, expected)
			}
		})
	}
}
