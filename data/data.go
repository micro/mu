package data

import (
	"os"
	"path/filepath"
)

func Save(key, val string) error {
	dir := os.ExpandEnv("$HOME/.mu")
	path := filepath.Join(dir, "data")
	file := filepath.Join(path, key)
	os.MkdirAll(path, 0700)
	os.WriteFile(file, []byte(val), 0644)
	return nil
}

func Load(key string) ([]byte, error) {
	dir := os.ExpandEnv("$HOME/.mu")
	path := filepath.Join(dir, "data")
	file := filepath.Join(path, key)
	return os.ReadFile(file)
}
