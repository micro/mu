package quota

// Prices reach this package from above — main embeds quota.json and calls Load.
// A test binary has no main, so without this every operation costs the 1-credit
// fallback and the tests below agree with each other about nothing.

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if err := LoadFromTree(); err != nil {
		panic("quota tests need quota.json: " + err.Error())
	}
	os.Exit(m.Run())
}
