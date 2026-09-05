package app

// A public conversation becomes the signed-in Home conversation rather than
// disappearing at the login boundary.

import (
	"strings"
	"testing"
)

func TestAChatCanHandItsTabStateToAnotherSurface(t *testing.T) {
	got := ChatComponent(ChatConfig{
		Ask:      true,
		StorageNS: "home",
		ImportNS:  "landing",
	})

	for _, want := range []string{
		`var IMPORT_NS="landing"`,
		"'mu_chat_'+from[n]+':'+IMPORT_NS",
		"sessionStorage.removeItem(source)",
		"pendingAgent.querySelector('.mu-think')",
		"sessionStorage.setItem(DKEY,user.textContent||'')",
		"sessionStorage.removeItem(TKEY)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the one-time chat handoff is missing %q", want)
		}
	}
}

func TestAStoredChatKeepsAnUnsentDraft(t *testing.T) {
	got := ChatComponent(ChatConfig{Ask: true, StorageNS: "landing"})

	for _, want := range []string{
		"mu_chat_draft:",
		"if(savedDraft)input.value=savedDraft",
		"saveDraft()",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the chat draft handoff is missing %q", want)
		}
	}
}
