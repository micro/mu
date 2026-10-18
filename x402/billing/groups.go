package billing

import (
	"fmt"
	"mu/internal/auth"
	"mu/internal/group"
	"net/http"
	"strings"
	"time"
)

// billingAccount resolves shared plans without creating a login for a group.
func billingAccount(id string) (*auth.Account, error) {
	if g, ok := group.FromScope(id); ok {
		return &auth.Account{ID: id, Name: g.Name, Created: g.Created}, nil
	}
	return auth.GetAccount(id)
}

func groupBudget(id string, amount int) error {
	if err := validGroupLedger(id); err != nil {
		return err
	}
	g, ok := group.FromScope(id)
	if !ok {
		return nil
	}
	used := groupSpent(id, time.Now().UTC())
	if amount > g.SpendLimit-used {
		return fmt.Errorf("the group's monthly spending limit has been reached; ask a group admin to adjust it")
	}
	return nil
}

// Called under the ledger lock, so concurrent reservations count immediately.
func groupSpent(id string, now time.Time) int {
	used := 0
	for _, tx := range transactions[id] {
		if tx == nil {
			continue
		}
		at := tx.CreatedAt
		if ref, ok := tx.Metadata["reservation"].(string); ok {
			for _, original := range transactions[id] {
				if original != nil && original.ID == ref {
					at = original.CreatedAt
					break
				}
			}
		}
		if at.UTC().Format("2006-01") != now.Format("2006-01") {
			continue
		}
		if tx.Type != TxSpend && tx.Type != TxRefund {
			continue
		}
		sign := 1
		if tx.Type == TxRefund {
			sign = -1
		}
		used -= tx.Amount
		for _, key := range []string{"monthly_credits", "daily_credits", "signup_credits"} {
			switch n := tx.Metadata[key].(type) {
			case int:
				used += sign * n
			case float64:
				used += sign * int(n)
			}
		}
	}
	return max(0, used)
}
func GroupSpent(id string) int {
	return withLedger(func(l *ledger) int { return groupSpent(id, time.Now().UTC()) })
}

// GroupSubscriptionSummary reuses the same checkout and plan implementation.
func GroupSubscriptionSummary(r *http.Request, g group.Group) string {
	acc, _ := billingAccount(group.Scope(g.ID))
	body := SubscriptionSummary(r, acc)
	body = strings.ReplaceAll(body, `action="/account/subscription"`, `action="/account/subscription?group=`+g.ID+`"`)
	body = strings.ReplaceAll(body, `/account?plan=`, `/groups?id=`+g.ID+`&tab=budget&plan=`)
	body = strings.ReplaceAll(body, `#subscription`, ``)
	return strings.ReplaceAll(body, "personal chat and tools", "shared chat and tools")
}
func subscriptionActor(r *http.Request, actor *auth.Account) (*auth.Account, string, error) {
	id := r.URL.Query().Get("group")
	if id == "" {
		return actor, "/account#subscription", nil
	}
	if !group.Admin(id, actor.ID) {
		return nil, "", fmt.Errorf("group not found")
	}
	acc, err := billingAccount(group.Scope(id))
	return acc, "/groups?id=" + id + "&tab=budget", err
}

// Keep the ledger locked until membership deletion commits: a simultaneous
// transfer must either fund an existing group or fail without debiting its payer.
func init() {
	group.GuardDeletion = func(remove func(check func(string) error) error) error {
		return withLedger(func(l *ledger) error {
			return remove(func(id string) error { return groupDeletionAllowed(l, id) })
		})
	}
}

func groupDeletionAllowed(l *ledger, id string) error {
	owner := group.Scope(id)
	if w := balances[owner]; w != nil && w.Balance > 0 {
		return fmt.Errorf("this group still has prepaid credits; use its balance or transfer group ownership before deleting it")
	}
	for _, tx := range transactions[owner] {
		if tx != nil && tx.Metadata["pending"] == true {
			return fmt.Errorf("wait for the group's pending usage to settle before deleting it")
		}
	}
	return nil
}

func validGroupLedger(id string) error {
	if strings.HasPrefix(id, "group:") {
		if _, ok := group.FromScope(id); !ok {
			return fmt.Errorf("group not found")
		}
	}
	return nil
}
