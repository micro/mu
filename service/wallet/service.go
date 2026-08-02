package wallet

import (
	"context"
	"fmt"

	"mu/internal/app"
	"mu/internal/service"
)

// Credits exposes the wallet as a service so other services can meter their own
// paid work without importing this package. A service that calls a provider we
// pay for checks quota first and charges after it succeeds — the same two-step
// Mu's own handlers use, reachable over RPC.
//
// The caller is never named in a request: identity comes from the call context
// (see internal/service/identity.go), so a service cannot spend someone else's
// credits by naming them.
type Credits struct{}

// ── Check ───────────────────────────────────────────────────────

type CheckRequest struct {
	Operation string `json:"operation" description:"The billable operation, e.g. \"web_search\""`
}

type CheckResponse struct {
	Allowed bool `json:"allowed" description:"True when the caller can afford this operation"`
	Cost    int  `json:"cost" description:"Credits the operation costs"`
	Balance int  `json:"balance" description:"The caller's current balance"`
}

// Check reports whether the caller can afford an operation, without charging.
// @example {"operation": "web_search"}
func (Credits) Check(ctx context.Context, req *CheckRequest, rsp *CheckResponse) error {
	who := service.AccountFrom(ctx)
	if who == "" {
		return fmt.Errorf("sign in to use credits")
	}
	if req.Operation == "" {
		return fmt.Errorf("operation is required")
	}
	ok, _, cost, err := CheckQuota(who, req.Operation)
	if err != nil {
		return err
	}
	rsp.Allowed, rsp.Cost, rsp.Balance = ok, cost, GetBalance(who)
	return nil
}

// ── Charge ──────────────────────────────────────────────────────

type ChargeRequest struct {
	Operation string `json:"operation" description:"The billable operation to charge for"`
}

type ChargeResponse struct {
	Charged int `json:"charged" description:"Credits deducted"`
	Balance int `json:"balance" description:"The caller's balance after the charge"`
}

// Charge deducts the cost of an operation from the caller's balance. Call it
// after the work succeeded, so a failure never bills.
// @example {"operation": "web_search"}
func (Credits) Charge(ctx context.Context, req *ChargeRequest, rsp *ChargeResponse) error {
	who := service.AccountFrom(ctx)
	if who == "" {
		return fmt.Errorf("sign in to use credits")
	}
	if req.Operation == "" {
		return fmt.Errorf("operation is required")
	}
	cost := GetOperationCost(req.Operation)
	if err := ConsumeQuota(who, req.Operation); err != nil {
		return err
	}
	rsp.Charged, rsp.Balance = cost, GetBalance(who)
	return nil
}

// ── Balance ─────────────────────────────────────────────────────

type BalanceRequest struct{}

type BalanceResponse struct {
	Balance int `json:"balance" description:"The caller's credit balance"`
}

// Balance returns the caller's current credit balance.
// @example {}
func (Credits) Balance(ctx context.Context, _ *BalanceRequest, rsp *BalanceResponse) error {
	who := service.AccountFrom(ctx)
	if who == "" {
		return fmt.Errorf("sign in to use credits")
	}
	rsp.Balance = GetBalance(who)
	return nil
}

// LoadService registers the wallet as a callable service. Named apart from
// Load, which already initialises the package's own state.
func LoadService() {
	if err := service.Register(Spec); err != nil {
		app.Log("wallet", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:        "wallet",
	Handler:     new(Credits),
	Description: "Credits: check, charge and balance",
	Page:        "/wallet",
	Scoped:      true,
	Icon:        "wallet.png",
	Endpoints: map[string]service.Endpoint{
		"Balance": {Doc: "Read the caller's current credit balance"},
		"Charge":  {Doc: "Deduct the cost of an operation from the caller's credit balance", Destructive: true},
		"Check":   {Doc: "Check whether the caller can afford an operation, without charging"},
	},
}
