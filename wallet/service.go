package wallet

import (
	"context"
	"fmt"

	"mu/internal/quota"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
)

// Credits exposes the wallet as a service so a caller can ask what it holds and
// what something costs before spending it. A service that calls a provider we
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
	Cost    int  `json:"cost" description:"Credits this caller will be charged — zero for an admin, or when payments are off"`
	Price   int  `json:"price" description:"What the operation is priced at on this instance, whoever is calling"`
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
	// "No" is an answer, not a failure.
	//
	// quota.CheckQuota returns an error when the caller cannot afford the operation,
	// which is the right shape for a gate about to refuse a call and the wrong
	// one here: this tool exists to be asked *before* spending, and returning
	// that error made it fail with "this costs 2 credits and your balance is
	// 0". So the one caller who most needs to know a price — somebody with no
	// credits, deciding whether to top up — was the one caller who could not
	// ask. Affordability is what the answer is about; it cannot also be the
	// reason there is no answer.
	// The account has to exist; that is the only genuine failure here. Both of
	// quota.CheckQuota's error cases return false, so they cannot be told apart by
	// its result — ask the question this one is about separately.
	if _, err := auth.GetAccount(who); err != nil {
		return fmt.Errorf("account not found")
	}
	ok, _, cost, _ := quota.CheckQuota(who, req.Operation)
	// Two different questions, and one number was answering both.
	//
	// Cost is what this caller pays, which quota.CheckQuota returns as zero for an
	// admin and on an instance with payments off. Price is what the operation
	// is priced at. An agent asking "what does web_search cost" was told zero
	// and could not tell the difference between free and free-for-you.
	rsp.Allowed, rsp.Cost, rsp.Balance = ok, cost, GetBalance(who)
	rsp.Price = quota.GetOperationCost(req.Operation)
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
	cost := quota.GetOperationCost(req.Operation)
	if err := quota.ConsumeQuota(who, req.Operation); err != nil {
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
// Load, which initialises the ledger this page and these tools read.
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
