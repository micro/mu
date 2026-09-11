package service

import (
	"context"
	"go-micro.dev/v6/metadata"
)

const restrictedKey = "Mu-Restricted-Caller"

// WithRestrictedCaller marks a grant narrower than the account. Durable work
// must not exchange that grant for the account's unrestricted agent authority.
func WithRestrictedCaller(ctx context.Context) context.Context {
	return metadata.Set(ctx, restrictedKey, "true")
}

func RestrictedCaller(ctx context.Context) bool {
	v, _ := metadata.Get(ctx, restrictedKey)
	return v == "true"
}
