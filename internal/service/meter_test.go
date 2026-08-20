package service

// What a cache hit costs the person who asked second.

import (
	"context"
	"testing"
)

// A handler that answered from something it already had is not charged for.
//
// The price of an operation is the price of going and getting it. service/weather
// keeps one forecast per place per half hour and serves it to everybody, and
// service/tiles fetches a map tile once, ever — so on a hit the instance pays
// nothing. It charged anyway, which handed the saving to the operator and made
// the second person to ask for London's weather pay the same as the first.
//
// That is the wrong way round for a shared instance: the reason to call a tool
// here rather than hold your own API key is that somebody may already have
// asked.
func TestACachedAnswerIsNotCharged(t *testing.T) {
	ctx, m := withMeter(context.Background())

	if m.servedFromCache() {
		t.Fatal("a call is charged by default and this one started free")
	}
	ServedFromCache(ctx)
	if !m.servedFromCache() {
		t.Error("the handler said it did not fetch anything and the meter did not hear")
	}
}

// Saying nothing means charge, which is the direction that fails safely.
//
// A handler that forgets costs a caller a credit — noticed, complained about,
// fixed. The inverse default would mean a handler that forgets gives the
// instance's money away silently, which is the failure nobody reports.
func TestSilenceMeansCharge(t *testing.T) {
	_, m := withMeter(context.Background())
	if m.servedFromCache() {
		t.Error("a handler that said nothing was treated as free")
	}
}

// And it is safe to say outside the gateway, where there is no meter: a page, a
// background loop, a test. Those are not charged either, so there is nothing to
// tell and nothing to panic about.
func TestSayingItWithoutAMeterIsHarmless(t *testing.T) {
	ServedFromCache(context.Background())
}

// Two goroutines answering parts of one call, either of which may have hit the
// cache. Run with -race.
func TestTheMeterSurvivesAFanOut(t *testing.T) {
	ctx, m := withMeter(context.Background())
	done := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		go func() { ServedFromCache(ctx); done <- struct{}{} }()
	}
	<-done
	<-done
	if !m.servedFromCache() {
		t.Error("the meter lost a report")
	}
}
