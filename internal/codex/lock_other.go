//go:build !linux

package codex

import (
	"context"
	"fmt"
)

func lock(context.Context) (func(), error) { return nil, fmt.Errorf("Codex preview requires Linux") }
