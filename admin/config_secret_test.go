package admin

// What must not be printed in full on a page somebody may screenshot.

import "testing"

func TestCredentialBearingURLsAreMasked(t *testing.T) {
	// Every hosted RPC provider puts the credential in the path, and nothing in
	// the variable's name says "key".
	for _, k := range []string{"BASE_RPC_URL", "TRADE_RPC_URL", "base_rpc_url"} {
		if !secret(k) {
			t.Errorf("%s renders in full, exposing the provider key in its path", k)
		}
	}
}

func TestOrdinaryValuesAreNotMasked(t *testing.T) {
	// Masking everything would make the page useless for the thing it is for.
	for _, k := range []string{"MAIL_DOMAIN", "MAIL_WHITELIST", "TRANSIT_FEEDS", "TRADE_CHAIN"} {
		if secret(k) {
			t.Errorf("%s is masked, but it holds nothing secret", k)
		}
	}
}

func TestTheChainSettingsAreReachable(t *testing.T) {
	for _, k := range []string{"BASE_RPC_URL", "TRADE_RPC_URL"} {
		if !Settable(k) {
			t.Errorf("%s cannot be set from /admin/config, so pointing this instance at "+
				"the right chain needs shell access", k)
		}
	}
}
