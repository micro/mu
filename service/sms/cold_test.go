package sms

// An account nobody has vouched for cannot text a stranger.
//
// Mail has had this rule for a while: what leaves here leaves under this
// instance's domain, so what an unaccountable account sends is charged to the
// deliverability of everybody else's mail. A number is the same shared thing —
// one reported for spam takes every other account's messages down with it —
// and there was no such rule here at all. Price and the daily limit are not
// it: they bound what one account spends, not what a hundred of them do.
//
// It became urgent when new accounts started arriving with a hundred credits,
// which is five texts to five strangers before the account has told us
// anything about itself.

import (
	"strings"
	"testing"

	"mu/internal/auth"
)

func TestAnUntrustedAccountCannotTextAStranger(t *testing.T) {
	setup(t)

	// The operator first. The first account on an instance is its admin, and an
	// admin is trusted — so a test that created only one account would be
	// testing the operator and would pass whatever this gate did.
	if err := auth.Create(&auth.Account{ID: "theoperator", Name: "theoperator"}); err != nil {
		t.Fatal(err)
	}

	const who = "coldcaller"
	if err := auth.Create(&auth.Account{ID: who, Name: who}); err != nil {
		t.Fatal(err)
	}
	if auth.Trusted(who) {
		t.Fatal("a bare new account reads as trusted, so this test would prove nothing")
	}

	_, err := Send(who, "+447700900321", "hello there")
	if err == nil {
		t.Fatal("a brand new account texted a number it has never heard from")
	}
	if !strings.Contains(err.Error(), "cannot message") {
		t.Fatalf("refused for the wrong reason: %v\n"+
			"The gate is the thing being tested; a refusal from further down the\n"+
			"function would pass this test while the gate was missing.", err)
	}
}

// And the gate is about the account, not about texting.
//
// A trusted account gets past it. There is no provider in a test so the send
// fails anyway — what matters is that it fails on something else.
func TestATrustedAccountIsNotStopped(t *testing.T) {
	setup(t)

	const who = "verifiedperson"
	if err := auth.Create(&auth.Account{ID: who, Name: who, EmailVerified: true}); err != nil {
		t.Fatal(err)
	}
	if !auth.Trusted(who) {
		t.Fatal("a verified address does not read as trusted")
	}

	_, err := Send(who, "+447700900322", "hello there")
	if err != nil && strings.Contains(err.Error(), "cannot message") {
		t.Errorf("a verified account was stopped by the cold-contact gate: %v", err)
	}
}
