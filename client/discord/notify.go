package discord

import (
	"fmt"

	"mu/internal/app"
)

// NotifyNewMail sends a DM when the user receives a new email.
// Called from the mail package via a callback.
func NotifyNewMail(accountID, from, subject, summary string) {
	if !Enabled() {
		return
	}
	if summary == "" {
		summary = subject
	}
	msg := fmt.Sprintf("📬 **New email from %s**\n%s", from, summary)
	NotifyUser(accountID, msg)
	app.Log("discord", "Notified %s about new email from %s", accountID, from)
}
