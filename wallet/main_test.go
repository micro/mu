package wallet

// Prices come from above: main embeds quota.json and hands it to internal/quota.
// A test binary has no main, so the cost tables here would render from the
// 1-credit fallback and every assertion about what something costs would be
// agreeing with itself about nothing.

import (
	"os"
	"testing"

	"mu/internal/quota"
)

func TestMain(m *testing.M) {
	if err := quota.LoadFromTree(); err != nil {
		panic("wallet tests need quota.json: " + err.Error())
	}
	os.Exit(m.Run())
}
