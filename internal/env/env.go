// Package env loads a dotenv file into the process environment at startup so
// that both `mu --serve` and one-off CLI commands (e.g. `mu x402`) see the same
// configuration — without relying on the shell, or on systemd's EnvironmentFile
// being applied. Values already present in the environment always win; the file
// only fills in what is unset.
//
// It is imported for its side effect by packages that read configuration at
// init time (e.g. wallet), guaranteeing the file is loaded before those reads.
package env

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

func init() { Load("") }

// Load reads KEY=VALUE lines from path and sets any key not already present in
// the environment. When path is empty it tries $MU_ENV_FILE, then ~/.env, then
// ~/.mu/.env — the first that exists is used. Missing files are ignored.
func Load(path string) {
	var paths []string
	if path != "" {
		paths = []string{path}
	} else {
		if p := strings.TrimSpace(os.Getenv("MU_ENV_FILE")); p != "" {
			paths = append(paths, p)
		}
		if home, err := os.UserHomeDir(); err == nil {
			paths = append(paths, filepath.Join(home, ".env"), filepath.Join(home, ".mu", ".env"))
		}
	}
	for _, p := range paths {
		if loadFile(p) {
			return // first existing file wins
		}
	}
}

// loadFile parses one dotenv file, returning true if it was read.
func loadFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.Trim(strings.TrimSpace(line[eq+1:]), `"'`)
		if key == "" {
			continue
		}
		if _, ok := os.LookupEnv(key); !ok {
			_ = os.Setenv(key, val)
		}
	}
	return true
}
