package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"go-micro.dev/v6/metadata"
	"sync"
	"time"
)

type operatorKey struct{}
type operatorGrant struct {
	account, target string
	expires         time.Time
}

var operatorGrants = struct {
	sync.Mutex
	values map[string]operatorGrant
}{values: make(map[string]operatorGrant)}

// WithOperator is used only after a trusted entrypoint authenticates an operator.
// It is a local context capability, never a caller-supplied metadata flag.
func WithOperator(ctx context.Context) context.Context {
	return context.WithValue(ctx, operatorKey{}, true)
}
func operatorCall(ctx context.Context, target string) (context.Context, func()) {
	if ctx.Value(operatorKey{}) != true || InAgentRun(ctx) {
		return ctx, func() {}
	}
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ctx, func() {}
	}
	token := hex.EncodeToString(b[:])
	operatorGrants.Lock()
	operatorGrants.values[token] = operatorGrant{AccountFrom(ctx), target, time.Now().Add(time.Minute)}
	operatorGrants.Unlock()
	ctx = metadata.Set(ctx, "Mu-Operator-Grant", token)
	return ctx, func() { operatorGrants.Lock(); delete(operatorGrants.values, token); operatorGrants.Unlock() }
}
func requireOperator(ctx context.Context, target string) error {
	token, _ := metadata.Get(ctx, "Mu-Operator-Grant")
	operatorGrants.Lock()
	grant, ok := operatorGrants.values[token]
	delete(operatorGrants.values, token)
	operatorGrants.Unlock()
	if !ok || InAgentRun(ctx) || grant.account == "" || grant.account != AccountFrom(ctx) || grant.target != target || time.Now().After(grant.expires) {
		return fmt.Errorf("operator authorization required")
	}
	return nil
}
