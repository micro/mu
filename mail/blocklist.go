package mail

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"mu/app"
	"mu/data"
)

// Blocklist for blocking abusive senders
type Blocklist struct {
	Emails []string `json:"emails"` // Blocked email addresses
	IPs    []string `json:"ips"`    // Blocked IP addresses
}

var (
	blocklistMutex sync.RWMutex
	blocklist      = &Blocklist{
		Emails: []string{},
		IPs:    []string{},
	}
)

// loadBlocklist loads the blocklist from disk
func loadBlocklist() {
	b, err := data.LoadFile("blocklist.json")
	if err != nil {
		app.Log("mail", "No blocklist file found, starting with empty blocklist")
		return
	}

	blocklistMutex.Lock()
	defer blocklistMutex.Unlock()

	if err := json.Unmarshal(b, blocklist); err != nil {
		app.Log("mail", "Error loading blocklist: %v", err)
		return
	}

	app.Log("mail", "Loaded blocklist: %d emails, %d IPs", len(blocklist.Emails), len(blocklist.IPs))
}

// saveBlocklist saves the blocklist to disk
func saveBlocklist() error {
	blocklistMutex.RLock()
	defer blocklistMutex.RUnlock()

	b, err := json.MarshalIndent(blocklist, "", "  ")
	if err != nil {
		return err
	}

	return data.SaveFile("blocklist.json", string(b))
}

// IsBlocked checks if an email or IP is blocked
func IsBlocked(email, ip string) bool {
	blocklistMutex.RLock()
	defer blocklistMutex.RUnlock()

	email = strings.ToLower(strings.TrimSpace(email))
	ip = strings.TrimSpace(ip)

	// Check email
	for _, blocked := range blocklist.Emails {
		if strings.ToLower(blocked) == email {
			return true
		}
		// Support wildcard domain blocking (e.g., "*@spam.com")
		if strings.HasPrefix(blocked, "*@") {
			domain := strings.TrimPrefix(blocked, "*@")
			if strings.HasSuffix(email, "@"+domain) {
				return true
			}
		}
	}

	// Check IP
	for _, blocked := range blocklist.IPs {
		if blocked == ip {
			return true
		}
	}

	return false
}

// BlockEmail adds an email to the blocklist
func BlockEmail(email string) error {
	blocklistMutex.Lock()
	defer blocklistMutex.Unlock()

	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return fmt.Errorf("email cannot be empty")
	}

	// Check if already blocked
	for _, blocked := range blocklist.Emails {
		if strings.ToLower(blocked) == email {
			return fmt.Errorf("email already blocked")
		}
	}

	blocklist.Emails = append(blocklist.Emails, email)
	app.Log("mail", "Blocked email: %s", email)

	return saveBlocklist()
}

// BlockIP adds an IP to the blocklist
func BlockIP(ip string) error {
	blocklistMutex.Lock()
	defer blocklistMutex.Unlock()

	ip = strings.TrimSpace(ip)
	if ip == "" {
		return fmt.Errorf("IP cannot be empty")
	}

	// Check if already blocked
	for _, blocked := range blocklist.IPs {
		if blocked == ip {
			return fmt.Errorf("IP already blocked")
		}
	}

	blocklist.IPs = append(blocklist.IPs, ip)
	app.Log("mail", "Blocked IP: %s", ip)

	return saveBlocklist()
}

// UnblockEmail removes an email from the blocklist
func UnblockEmail(email string) error {
	blocklistMutex.Lock()
	defer blocklistMutex.Unlock()

	email = strings.ToLower(strings.TrimSpace(email))

	for i, blocked := range blocklist.Emails {
		if strings.ToLower(blocked) == email {
			blocklist.Emails = append(blocklist.Emails[:i], blocklist.Emails[i+1:]...)
			app.Log("mail", "Unblocked email: %s", email)
			return saveBlocklist()
		}
	}

	return fmt.Errorf("email not found in blocklist")
}

// UnblockIP removes an IP from the blocklist
func UnblockIP(ip string) error {
	blocklistMutex.Lock()
	defer blocklistMutex.Unlock()

	ip = strings.TrimSpace(ip)

	for i, blocked := range blocklist.IPs {
		if blocked == ip {
			blocklist.IPs = append(blocklist.IPs[:i], blocklist.IPs[i+1:]...)
			app.Log("mail", "Unblocked IP: %s", ip)
			return saveBlocklist()
		}
	}

	return fmt.Errorf("IP not found in blocklist")
}

// GetBlocklist returns a copy of the current blocklist
func GetBlocklist() *Blocklist {
	blocklistMutex.RLock()
	defer blocklistMutex.RUnlock()

	return &Blocklist{
		Emails: append([]string{}, blocklist.Emails...),
		IPs:    append([]string{}, blocklist.IPs...),
	}
}
