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
	for _, scenario := range []string{"success", "corrupt", "cannot-run", "restart-fails", "late-exit", "restart-loop"} {
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
 show)
  if [ "$2" = --property=MainPID ]; then
   if [ "$TEST_SCENARIO" = restart-loop ] && test -e "$TEST_LOG.pid"; then echo 456; else echo 123; fi
   touch "$TEST_LOG.pid"
   exit 0
  fi
  printf '{ path=%s ; argv[]=%s --serve ; }\n' "$TEST_BINARY" "$TEST_BINARY" ;;
 restart)
  echo restart >> "$TEST_LOG"
  if [ "$TEST_SCENARIO" = restart-fails ] && ! test -e "$TEST_LOG.failed"; then
   touch "$TEST_LOG.failed"
   exit 1
  fi ;;
 is-active)
  if [ "$TEST_SCENARIO" = late-exit ] && test -e "$TEST_LOG.active"; then exit 1; fi
  touch "$TEST_LOG.active"
  exit 0 ;;
 *) exit 99 ;;
esac
`)
			write(filepath.Join(mocks, "sleep"), "#!/bin/sh\nexit 0\n")
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
			if scenario == "restart-fails" || scenario == "late-exit" || scenario == "restart-loop" {
				expected = 2
			}
			if count != expected {
				t.Fatalf("restart count %d, want %d", count, expected)
			}
		})
	}
}

func TestDeployArchitectureOutput(t *testing.T) {
	workflow, err := os.ReadFile(at(".github/workflows/deploy.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	start := strings.Index(text, "          architecture=$(printf")
	if start < 0 {
		t.Fatal("missing architecture parser")
	}
	end := strings.Index(text[start:], "\n\n")
	if end < 0 {
		t.Fatal("missing parser end")
	}
	script := "set -e\n" + text[start:start+end]
	for _, tt := range []struct{ name, input, want string }{
		{"bare", "MU_DEPLOY_ARCH=amd64\n", "arch=amd64\n"},
		{"formatted", "======CMD======\nprintf 'MU_DEPLOY_ARCH=%s\\n' \"$architecture\"\n======END======\nout: MU_DEPLOY_ARCH=arm64\r\nSuccessfully executed commands\n", "arch=arm64\n"},
		{"unknown", "MU_DEPLOY_ARCH=unknown\n", ""},
		{"duplicate", "MU_DEPLOY_ARCH=amd64\nMU_DEPLOY_ARCH=arm64\n", ""},
		{"noise", "amd64\n", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "output")
			cmd := exec.Command("sh", "-c", script)
			cmd.Env = append(os.Environ(), "SSH_OUTPUT="+tt.input, "GITHUB_OUTPUT="+output)
			log, err := cmd.CombinedOutput()
			if (err == nil) != (tt.want != "") {
				t.Fatalf("unexpected result %v: %s", err, log)
			}
			got, _ := os.ReadFile(output)
			if string(got) != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
