package app

// EnvInt lives here, and the guest limiter that used to does not.
//
// It was one ceiling keyed on an address, and an address is both too coarse
// (a cafe is one address and a hundred people) and too easily changed (the
// header naming it was believed from anybody). Both halves are in client.go
// now, next to the question they answer. This file is what was left.

import (
	"fmt"
	"os"
)

func EnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return def
}
