package mail

import (
	"mu/service/mail"
	"strings"
)

// Admission facts come from the authenticated transport. Deciding whether this
// particular application responds belongs here, not in the mail service.
func acceptedInstruction(m mail.InboundMail, facts map[string]interface{}) bool {
	authenticated, _ := facts["authenticated"].(bool)
	owned, _ := facts["owned"].(bool)
	machine, _ := facts["machine"].(bool)
	if !authenticated || machine || !m.Shared || m.Tag != "" || strings.EqualFold(m.From, m.To) {
		return false
	}
	local, domain, ok := strings.Cut(strings.ToLower(m.From), "@")
	local, _, _ = strings.Cut(local, "+")
	if ok && strings.EqualFold(domain, mail.ConfiguredDomain()) && (local == "agent" || local == "hello") {
		return false
	}
	return owned || mail.SenderIsAccountOwner(m.Owner, m.From)
}
