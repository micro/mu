package mail

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"os"
	"sort"
	"strings"
	"sync"

	"mu/internal/data"
)

// Personal delivery preferences never grant permission to invoke an agent.
var senderRules struct {
	sync.Mutex
	loaded   bool
	err      error
	accounts map[string][]string
}

func loadSenderRules() error {
	if senderRules.loaded {
		return senderRules.err
	}
	senderRules.loaded = true
	senderRules.accounts = map[string][]string{}
	b, err := data.LoadFile("mail_allowed_senders.json")
	if os.IsNotExist(err) {
		return nil
	}
	if err == nil {
		err = json.Unmarshal(b, &senderRules.accounts)
	}
	if err != nil {
		senderRules.err = fmt.Errorf("allowed senders are unavailable")
	}
	if senderRules.accounts == nil {
		senderRules.accounts = map[string][]string{}
	}
	return senderRules.err
}

func senderRule(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.Contains(value, "@") && !strings.HasPrefix(value, "@") {
		a, err := mail.ParseAddress(value)
		if err != nil || a.Address != value {
			return "", fmt.Errorf("enter an email address or domain")
		}
		_, domain, _ := strings.Cut(value, "@")
		if !validSenderDomain(domain) {
			return "", fmt.Errorf("enter a full domain such as peerlist.io")
		}
		return value, nil
	}
	domain := strings.TrimPrefix(value, "@")
	if !validSenderDomain(domain) {
		return "", fmt.Errorf("enter an email address or full domain such as peerlist.io")
	}
	return "@" + domain, nil
}

func validSenderDomain(domain string) bool {
	if len(domain) > 253 || !strings.Contains(domain, ".") {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

func allowedSenders(owner string) ([]string, error) {
	senderRules.Lock()
	defer senderRules.Unlock()
	if err := loadSenderRules(); err != nil {
		return nil, err
	}
	return append([]string(nil), senderRules.accounts[owner]...), nil
}

func changeAllowedSender(owner, value string, remove bool) error {
	if owner == "" {
		return fmt.Errorf("sign in to manage allowed senders")
	}
	rule, err := senderRule(value)
	if err != nil {
		return err
	}
	senderRules.Lock()
	defer senderRules.Unlock()
	if err := loadSenderRules(); err != nil {
		return err
	}
	next := map[string][]string{}
	for id, values := range senderRules.accounts {
		next[id] = append([]string(nil), values...)
	}
	values := []string{}
	for _, v := range next[owner] {
		if v != rule {
			values = append(values, v)
		}
	}
	if !remove {
		values = append(values, rule)
	}
	if len(values) > 200 {
		return fmt.Errorf("you can allow up to 200 senders or domains")
	}
	sort.Strings(values)
	next[owner] = values
	if err := data.SaveJSON("mail_allowed_senders.json", next); err != nil {
		return err
	}
	senderRules.accounts = next
	return nil
}

func personalSenderAllowed(owner, from string) bool {
	values, err := allowedSenders(owner)
	if err != nil {
		return false
	}
	from = strings.ToLower(strings.TrimSpace(from))
	_, domain, ok := strings.Cut(from, "@")
	if !ok {
		return false
	}
	for _, rule := range values {
		if rule == from || rule == "@"+domain {
			return true
		}
	}
	return false
}

// classifyInbound is per recipient. An authenticated personal allow entry
// bypasses heuristic scoring only; SMTP blocklists and spoof checks run first.
func classifyInbound(owner, from string, aligned bool, reason string, result SpamResult) SpamResult {
	if personalSenderAllowed(owner, from) {
		if aligned {
			return SpamResult{}
		}
		result.IsSpam = true
		result.Reasons = append(result.Reasons, "allowed sender could not be authenticated")
	} else if reason != "" {
		result.IsSpam = true
		result.Reasons = append(result.Reasons, reason)
	}
	return result
}

func forgetAllowedSenders(owner string) error {
	senderRules.Lock()
	defer senderRules.Unlock()
	if err := loadSenderRules(); err != nil {
		return err
	}
	next := map[string][]string{}
	for id, values := range senderRules.accounts {
		if id != owner {
			next[id] = values
		}
	}
	if err := data.SaveJSON("mail_allowed_senders.json", next); err != nil {
		return err
	}
	senderRules.accounts = next
	return nil
}
