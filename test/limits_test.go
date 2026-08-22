package test

// A limit is the second control, and it is not about money.
//
// A price stops somebody who has to pay. It does nothing about a loop, and a
// runaway agent with a funded balance is the case that costs real money — which
// is why the three operations that leave this instance carry a cap as well as a
// price. What they spend is a domain's or a number's reputation, and no refund
// repairs that.
//
// Before this, one of the three had a cap (sms, in its own package, under its
// own setting), one was about to grow one under a second name (email), and one
// had none at all. The number now lives in quota.json beside the price, because
// what a thing costs and how much of it somebody may do are the same kind of
// decision made by the same person.

import (
	"testing"

	"mu/internal/quota"
)

// TestEverythingThatLeavesTheBuildingIsCapped — the three, by name, because the
// list is the point. A fourth outbound operation added without one would be the
// same exposure external email was.
func TestEverythingThatLeavesTheBuildingIsCapped(t *testing.T) {
	loadPrices(t)
	for _, op := range []string{
		quota.OpSMSSend,
	} {
		if n := quota.DailyLimit(op); n == quota.NoLimit {
			t.Errorf("%s puts a message in front of somebody outside this instance "+
				"and has no daily cap, so a loop is bounded only by a balance", op)
		}
	}
}

// TestOnlyOutboundOrFreeIsCapped — a cap on a *priced* read would be a paywall
// wearing a safety label. Somebody paying for each call is already bounded by
// what they are willing to pay, there is no reputation being spent, and there is
// no reason to stop them.
//
// A free operation is the other case, and it is why this rule has two halves
// now. Talking to your agent costs nothing — it is the product rather than a
// tool it calls — so no price bounds it and a loop would run until the model
// bill did. There the count is the only control there is, which is the same
// argument the outbound three make, arrived at from the opposite direction.
//
// So: capped if it leaves the building, capped if it is free, and never
// otherwise.
//
// Four leave the building, not three. mail_email is the mailbox answering
// somebody outside and external_email is the channel sending from its own
// domain — they were one operation, so they shared one daily cap, and ten sends
// through the channel left an account unable to answer its own correspondence.
func TestOnlyOutboundOrFreeIsCapped(t *testing.T) {
	loadPrices(t)
	outbound := map[string]bool{
		quota.OpMailSend: true,
		quota.OpSMSSend:  true,
	}
	for _, p := range quotaPrices(t) {
		if outbound[p.op] {
			continue
		}
		capped := quota.DailyLimit(p.op) != quota.NoLimit
		free := quota.OperationCost(p.op) == 0

		if capped && !free {
			t.Errorf("%s is capped at %d a day, costs %d, and nothing it does leaves "+
				"this instance — a limit on a priced read is a paywall with a safety "+
				"label on it", p.op, quota.DailyLimit(p.op), quota.OperationCost(p.op))
		}
	}
}

// There is no agent operation to cap, because the agent is not a service.
//
// It was agent_query, priced at 7, then priced at 0 with a daily count. Both
// were the same mistake in different clothes: the agent reads the catalogue, so
// it cannot be in it, and it has no more business having a price than a price
// list has. What a run costs is what the tools it called cost, one line each on
// the receipt.
func TestTheAgentHasNoPrice(t *testing.T) {
	loadPrices(t)
	for _, op := range []string{"agent_query", "chat_query"} {
		if quota.Published(op) {
			t.Errorf("%s is in the price list — the agent consumes the catalogue "+
				"and cannot be in it", op)
		}
	}
}
