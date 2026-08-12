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
	"mu/wallet"
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

// TestAPlanRaisesTheSendCaps — this is the third thing a plan sells, after the
// credits and the agents, and the only one whose cost is not covered by the
// credits.
func TestAPlanRaisesTheSendCaps(t *testing.T) {
	loadPrices(t)
	floor := quota.DailyLimit(quota.OpExternalEmail)
	for _, p := range wallet.Plans() {
		n, ok := p.LimitFor(quota.OpExternalEmail)
		if !ok {
			t.Errorf("plan %q says nothing about how much it may send, so it sells "+
				"nothing on the one axis whose cost the credits do not cover", p.ID)
			continue
		}
		if n <= floor {
			t.Errorf("plan %q allows %d emails a day and everybody gets %d",
				p.ID, n, floor)
		}
	}
}

// TestThePlansAscendOnEveryAxisTheySell — a dearer plan that allows less of
// something is a bug you can only find by reading two cards side by side.
func TestThePlansAscendOnEveryAxisTheySell(t *testing.T) {
	plans := wallet.Plans()
	for i := 1; i < len(plans); i++ {
		prev, cur := plans[i-1], plans[i]
		for _, op := range []string{quota.OpExternalEmail, quota.OpSMSSend, quota.OpWhatsAppSend} {
			a, aok := prev.LimitFor(op)
			b, bok := cur.LimitFor(op)
			if !aok || !bok {
				continue
			}
			if b < a {
				t.Errorf("%s costs more than %s and allows %d of %s against %d",
					cur.ID, prev.ID, b, op, a)
			}
		}
	}
}

// TestPayAsYouGoIsNotAPlanButIsNotNothing — the column that replaced the third
// tier. It has to answer for an account with no subscription, and it must not
// answer zero.
func TestPayAsYouGoIsNotAPlanButIsNotNothing(t *testing.T) {
	none := wallet.PlanByID("")
	if none.Agents < 1 {
		t.Errorf("somebody paying as they go may run %d agents", none.Agents)
	}
	if _, ok := none.LimitFor(quota.OpExternalEmail); ok {
		t.Error("pay as you go carries its own send caps, which means quota.json's " +
			"floor is no longer the floor and there are two places to change it")
	}
}

// TestACountedCallIsCountedOnce — the allowance and the limit read one counter,
// and it moves after the call rather than before, so nothing that failed spends
// somebody's allowance.
func TestACountedCallIsCountedOnce(t *testing.T) {
	quota.ResetAllowances()
	const who, op = "counted-once", "test_counting_op"

	if n := quota.UsedToday(who, op); n != 0 {
		t.Fatalf("a fresh counter starts at %d", n)
	}
	quota.Done(who, op)
	quota.Done(who, op)
	if n := quota.UsedToday(who, op); n != 2 {
		t.Errorf("two calls counted as %d", n)
	}
	quota.ResetAllowances()
	if n := quota.UsedToday(who, op); n != 0 {
		t.Errorf("after a reset the counter reads %d", n)
	}
}
