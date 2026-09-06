package service

import (
	"context"
	"go-micro.dev/v6/metadata"
	"testing"
)

func TestOperatorGrantIsBoundAndSingleUse(t *testing.T) {
	ctx := WithOperator(WithAccount(context.Background(), "operator"))
	ctx, done := operatorCall(ctx, "probe.Status")
	defer done()
	if err := requireOperator(ctx, "probe.Status"); err != nil {
		t.Fatal(err)
	}
	if requireOperator(ctx, "probe.Status") == nil {
		t.Fatal("replay accepted")
	}
	ctx, done = operatorCall(WithOperator(WithAccount(context.Background(), "operator")), "probe.Status")
	defer done()
	if requireOperator(ctx, "probe.Restart") == nil {
		t.Fatal("wrong operation accepted")
	}
	forged := metadata.Set(WithAccount(context.Background(), "operator"), "Mu-Operator-Grant", "true")
	if requireOperator(forged, "probe.Status") == nil {
		t.Fatal("forged grant accepted")
	}
	ctx, done = operatorCall(WithAgentRun(WithOperator(WithAccount(context.Background(), "operator"))), "probe.Status")
	defer done()
	if requireOperator(ctx, "probe.Status") == nil {
		t.Fatal("agent accepted")
	}
}
