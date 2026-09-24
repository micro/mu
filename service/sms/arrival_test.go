package sms

import (
	"context"
	"mu/internal/phone"
	"mu/internal/service"
	"testing"
)

func TestInboundProviderRetryKeepsOneSourceAndDoesNotInventTrust(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := receiveOn(ChannelSMS, "arrival-sms", "+447700900123", "private text", 1, "SM-retry-test")
	b := receiveOn(ChannelSMS, "arrival-sms", "+447700900123", "private text", 1, "SM-retry-test")
	if a.ID == "" || a.ID != b.ID {
		t.Fatalf("duplicate or unsaved: %q %q", a.ID, b.ID)
	}
	for _, owner := range []string{"arrival-sms", "other"} {
		var rsp service.SourceResponse
		if err := (Server{}).Source(service.WithAccount(context.Background(), owner), &service.SourceRequest{ID: a.ID}, &rsp); err != nil {
			t.Fatal(err)
		}
		if owner == "other" {
			if rsp.Item != nil {
				t.Fatal("cross-account SMS")
			}
			continue
		}
		if rsp.Item == nil || rsp.Item.Facts["verified_owner"] != false {
			t.Fatal("unknown correspondent can invoke assistant")
		}
	}
}

func TestOnlyAuthenticatedArrivalRecordsOwnerTrust(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const owner = "verified-arrival"
	const number = "+447700900321"
	if err := phone.Verify(owner, number); err != nil {
		t.Fatal(err)
	}
	imported := recordOn(ChannelSMS, owner, "in", number, "imported", 1, "")
	received := receiveOn(ChannelSMS, owner, number, "received", 1, "SM-verified")
	for _, tc := range []struct {
		id   string
		want bool
	}{{imported.ID, false}, {received.ID, true}} {
		var rsp service.SourceResponse
		if err := (Server{}).Source(service.WithAccount(context.Background(), owner), &service.SourceRequest{ID: tc.id}, &rsp); err != nil {
			t.Fatal(err)
		}
		if rsp.Item == nil || rsp.Item.Facts["verified_owner"] != tc.want {
			t.Fatalf("%+v", rsp.Item)
		}
	}
}
