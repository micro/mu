package mail

import (
	"mu/internal/auth"
	"testing"
)

func TestIMAPRecipientNameKeepsTaggedAddress(t *testing.T) {
	const owner = "recipient_name_test"
	if err := auth.Create(&auth.Account{ID: owner, Name: "Recipient_Name_Test"}); err != nil {
		t.Fatal(err)
	}
	defer auth.DeleteAccount(owner)
	for _, tag := range []string{"", "brief", "checkin"} {
		m := &Message{ToID: owner, To: owner, Tag: tag}
		name, address := imapDelivered(m)
		local := owner
		if tag != "" {
			local += "+" + tag
		}
		if name != "Recipient_Name_Test" || address != EmailForUser(local, ConfiguredDomain()) {
			t.Fatalf("tag %q: name=%q address=%q", tag, name, address)
		}
	}
}
