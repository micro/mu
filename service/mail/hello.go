package mail

import (
	"bytes"
	"fmt"
	"mu/internal/auth"
	"strings"
)

// Introduction hooks are wired by the server; mail does not import agents.
var Introduction func(email, id, subject, text string) error
var ClaimIntroduction func(owner, email string) error

// HelloAddress does not take over an existing account's mailbox.
func HelloAddress() string {
	if acc, _ := auth.GetAccount("hello"); acc != nil {
		return ""
	}
	if SharedAgentAddress() == "" {
		return ""
	}
	return "hello@" + ConfiguredDomain()
}
func sharedMailbox(local string) bool {
	return strings.EqualFold(local, AgentMailbox) || (strings.EqualFold(local, "hello") && HelloAddress() != "")
}
func helloRecipient(raw string) bool {
	return strings.EqualFold(strings.TrimSpace(raw), HelloAddress()) && HelloAddress() != ""
}

// SendIntroductionReply does not create an outbound relationship or whitelist.
func SendIntroductionReply(to, subject, body, inReplyTo, messageID string) error {
	from := HelloAddress()
	if from == "" {
		return fmt.Errorf("hello address unavailable")
	}
	message, generated := buildExternal("Micro", from, "", to, "Re: "+strings.TrimPrefix(subject, "Re: "), body, "", inReplyTo, "")
	message = bytes.Replace(message, []byte("Message-ID: "+generated+"\r\n"), []byte("Message-ID: "+messageID+"\r\n"), 1)
	message = append([]byte("Auto-Submitted: auto-replied\r\nX-Auto-Response-Suppress: All\r\n"), message...)
	return RelayToExternal("", to, signExternal(message))
}
