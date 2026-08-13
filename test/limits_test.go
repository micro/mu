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
		quota.OpExternalEmail,
		quota.OpSMSSend,
		quota.OpWhatsAppSend,
	} {
		if n := quota.DailyLimit(op); n == quota.NoLimit {
			t.Errorf("%s puts a message in front of somebody outside this instance "+
				"and has no daily cap, so a loop is bounded only by a balance", op)
		}
	}
}

// TestOnlyOutboundIsCapped — a cap on a read would be a paywall wearing a
// safety label. Reading costs credits and nothing else; there is no reputation
// to spend and no reason to stop somebody who is paying.
func TestOnlyOutboundIsCapped(t *testing.T) {
	loadPrices(t)
	outbound := map[string]bool{
		quota.OpExternalEmail: true,
		quota.OpSMSSend:       true,
		quota.OpWhatsAppSend:  true,
	}
	for _, p := range quotaPrices(t) {
		if outbound[p.op] {
			continue
		}
		if n := quota.DailyLimit(p.op); n != quota.NoLimit {
			t.Errorf("%s is capped at %d a day and nothing it does leaves this "+
				"instance — a limit on a read is a paywall with a safety label on it",
				p.op, n)
		}
	}
}
