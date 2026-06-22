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

// NotifyTradeConfirmed sends a DM when a trade is confirmed on-chain.
func NotifyTradeConfirmed(accountID, fromToken, toToken, amountIn, amountOut, txHash string) {
	if !Enabled() {
		return
	}
	msg := fmt.Sprintf("✅ **Trade confirmed**\n%s → %s\n[View on explorer](%s/tx/%s)",
		amountIn, amountOut, "https://etherscan.io", txHash)
	NotifyUser(accountID, msg)
}
