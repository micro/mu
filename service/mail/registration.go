package mail

import (
	"crypto/sha256"
	"fmt"
	"net/mail"
	"strings"
	"sync"
	"time"

	"mu/internal/origin"
	"mu/internal/userdb"
)

var registrationMu sync.Mutex
var registrationSending sync.Map

func sharedRecipient(raw string) bool {
	a, err := mail.ParseAddress(raw)
	if err != nil {
		return false
	}
	local, domain, ok := strings.Cut(a.Address, "@")
	return ok && strings.EqualFold(local, AgentMailbox) && strings.EqualFold(domain, ConfiguredDomain())
}

// Registration replies never whitelist a sender or create an account. Only
// authenticated, non-automatic mail addressed solely to agent@ reaches here.
// The persisted budget bounds attempts, including failures, across restarts.
func sendRegistrationReply(to, messageID string) error {
	base := origin.Self()
	if base == "" {
		return fmt.Errorf("public origin is not configured")
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ToLower(to))))
	if _, busy := registrationSending.LoadOrStore(key, true); busy {
		return fmt.Errorf("registration reply already in progress")
	}
	defer registrationSending.Delete(key)
	allowed, err := registrationBudget(key, false)
	if err != nil || !allowed {
		return err
	}
	body := "Welcome to Micro.\n\nCreate an account or sign in at " + base + "/account/connections, then verify the email address you are writing from.\n\nOnce connected, send your message again to " + SharedAgentAddress() + " to speak to your assistant. Your original request has not been processed."
	message, _ := buildExternal("Micro", SharedAgentAddress(), "", to, "Get started with Micro", body, "", messageID, "")
	message = append([]byte("Auto-Submitted: auto-replied\r\nX-Auto-Response-Suppress: All\r\n"), message...)
	// Null envelope sender prevents bounces from starting another exchange.
	// Bypass finishExternal: onboarding must not record an outbound relationship.
	if err := RelayToExternal("", to, signExternal(message)); err != nil {
		return err
	}
	_, err = registrationBudget(key, true)
	return err
}

func registrationBudget(key string, delivered bool) (bool, error) {
	registrationMu.Lock()
	defer registrationMu.Unlock()
	const ns, owner, collection = "mail", "instance", "registration"
	records, err := userdb.List(ns, owner, collection, "mine", nil, "", "", 1)
	if err != nil {
		return false, err
	}
	day := time.Now().UTC().Format("2006-01-02")
	state := map[string]interface{}{"day": day, "attempts": 0}
	senders := map[string]interface{}{}
	if len(records) > 0 && records[0].Data["day"] == day {
		state = records[0].Data
		if saved, ok := state["senders"].(map[string]interface{}); ok {
			for k, v := range saved {
				senders[k] = v
			}
		}
	}
	attempts := 0
	switch n := state["attempts"].(type) {
	case int:
		attempts = n
	case float64:
		attempts = int(n)
	}
	if delivered {
		senders[key] = true
	} else {
		if done, _ := senders[key].(bool); done {
			return false, nil
		}
		if attempts >= 200 {
			return false, fmt.Errorf("registration reply budget exhausted")
		}
		// Reserve an attempt before sending. Failed deliveries can be retried
		// by the originating mail server, but every attempt uses the budget.
		attempts++
	}
	state = map[string]interface{}{"day": day, "attempts": attempts, "senders": senders}
	if len(records) == 0 {
		_, err = userdb.Create(ns, owner, collection, state, false)
	} else {
		_, err = userdb.Update(ns, owner, collection, records[0].ID, state, false)
	}
	return err == nil, err
}
