package mail

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"

	"mu/internal/app"
	"mu/internal/data"
)

// SpamResult holds the outcome of a spam check
type SpamResult struct {
	IsSpam  bool     // Whether the message is classified as spam
	Score   int      // Total spam score (higher = more likely spam)
	Reasons []string // Which rules triggered
}

// SpamFilter configuration
type SpamFilter struct {
	Enabled          bool     `json:"enabled"`            // Master switch
	Threshold        int      `json:"threshold"`          // Score threshold for spam (default 5)
	BlockedTLDs      []string `json:"blocked_tlds"`       // Blocked top-level domains (e.g., ".vn", ".xyz")
	BlockedKeywords  []string `json:"blocked_keywords"`   // Blocked keywords in subject/body
	AllowedSenders   []string `json:"allowed_senders"`    // Whitelisted email addresses or domains
	RejectSpam       bool     `json:"reject_spam"`        // Reject at SMTP level (true) or silently drop (false)
	AutoBlockDomains bool     `json:"auto_block_domains"` // Auto-add spam sender domains to blocklist
}

var (
	spamMutex  sync.RWMutex
	spamFilter = &SpamFilter{
		Enabled:          true,
		Threshold:        5,
		BlockedTLDs:      []string{},
		BlockedKeywords:  []string{},
		AllowedSenders:   []string{},
		RejectSpam:       true,
		AutoBlockDomains: false,
	}
)

// Common spam patterns compiled once
var (
	excessiveCapsRe  = regexp.MustCompile(`[A-Z]{10,}`)
	urlPattern       = regexp.MustCompile(`https?://[^\s<>"]+`)
	suspiciousURLRe  = regexp.MustCompile(`https?://\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`)
	homoglyphPattern = regexp.MustCompile(`[а-яА-Я]`) // Cyrillic characters mixed with Latin
)

// Spam keyword patterns (case-insensitive matching applied at check time)
var spamPhrases = []string{
	"act now", "click here", "click below",
	"congratulations you", "dear winner", "you have been selected",
	"claim your prize", "free gift", "winner notification",
	"wire transfer", "bank transfer", "western union",
	"nigerian prince", "inheritance fund",
	"100% free", "no cost", "risk free",
	"unsubscribe", "opt out", "opt-out",
	"viagra", "cialis", "pharmacy",
	"weight loss", "lose weight",
	"casino", "lottery", "jackpot",
	"make money fast", "earn extra cash", "work from home",
	"invoice attached", "payment overdue", "account suspended",
	"verify your account", "confirm your identity",
	"limited time offer", "expires today", "urgent response",
}

// loadSpamFilter loads spam filter config from disk
func loadSpamFilter() {
	b, err := data.LoadFile("spamfilter.json")
	if err != nil {
		app.Log("mail", "No spam filter config found, using defaults")
		return
	}

	spamMutex.Lock()
	defer spamMutex.Unlock()

	if err := json.Unmarshal(b, spamFilter); err != nil {
		app.Log("mail", "Error loading spam filter config: %v", err)
		return
	}

	app.Log("mail", "Loaded spam filter: enabled=%v threshold=%d blocked_tlds=%d keywords=%d allowed=%d",
		spamFilter.Enabled, spamFilter.Threshold,
		len(spamFilter.BlockedTLDs), len(spamFilter.BlockedKeywords), len(spamFilter.AllowedSenders))
}

// saveSpamFilter persists spam filter config (caller must hold spamMutex)
func saveSpamFilter() error {
	b, err := json.MarshalIndent(spamFilter, "", "  ")
	if err != nil {
		return err
	}
	return data.SaveFile("spamfilter.json", string(b))
}

// CheckSpam evaluates an inbound message for spam signals.
// spfPass/dkimPass indicate whether SPF/DKIM verification succeeded.
// Returns a SpamResult with score and reasons.
func CheckSpam(from, subject, body, ip string, spfPass, dkimPass bool) SpamResult {
	spamMutex.RLock()
	defer spamMutex.RUnlock()

	result := SpamResult{}

	if !spamFilter.Enabled {
		return result
	}

	fromLower := strings.ToLower(from)
	subjectLower := strings.ToLower(subject)
	bodyLower := strings.ToLower(body)
	combined := subjectLower + " " + bodyLower

	// Check allowlist first — skip scoring for trusted senders
	for _, allowed := range spamFilter.AllowedSenders {
		allowed = strings.ToLower(allowed)
		if allowed == fromLower {
			return result
		}
		if strings.HasPrefix(allowed, "@") && strings.HasSuffix(fromLower, allowed) {
			return result
		}
	}

	// --- Sender-based checks ---

	// Extract sender domain
	senderDomain := ""
	if parts := strings.SplitN(fromLower, "@", 2); len(parts) == 2 {
		senderDomain = parts[1]
	}

	// TLD check
	if senderDomain != "" {
		for _, tld := range spamFilter.BlockedTLDs {
			tld = strings.ToLower(strings.TrimSpace(tld))
			if !strings.HasPrefix(tld, ".") {
				tld = "." + tld
			}
			if strings.HasSuffix(senderDomain, tld) {
				result.Score += 4
				result.Reasons = append(result.Reasons, fmt.Sprintf("blocked TLD: %s", tld))
			}
		}
	}

	// SPF check
	if !spfPass {
		result.Score += 3
		result.Reasons = append(result.Reasons, "SPF verification failed")
	}

	// DKIM check
	if !dkimPass {
		result.Score += 3
		result.Reasons = append(result.Reasons, "DKIM verification failed")
	}

	// Reverse DNS mismatch — no PTR record for sending IP
	if ip != "" {
		names, err := net.LookupAddr(ip)
		if err != nil || len(names) == 0 {
			result.Score += 2
			result.Reasons = append(result.Reasons, "no reverse DNS (PTR) for sender IP")
		} else if senderDomain != "" {
			// Check if any PTR record relates to the sender domain
			matched := false
			for _, name := range names {
				if strings.Contains(strings.ToLower(name), senderDomain) {
					matched = true
					break
				}
			}
			if !matched {
				result.Score += 1
				result.Reasons = append(result.Reasons, "reverse DNS does not match sender domain")
			}
		}
	}

	// --- Content-based checks ---

	// Built-in spam phrases
	for _, phrase := range spamPhrases {
		if strings.Contains(combined, phrase) {
			result.Score += 2
			result.Reasons = append(result.Reasons, fmt.Sprintf("spam phrase: %q", phrase))
		}
	}

	// Admin-configured keywords
	for _, kw := range spamFilter.BlockedKeywords {
		if strings.Contains(combined, strings.ToLower(kw)) {
			result.Score += 3
			result.Reasons = append(result.Reasons, fmt.Sprintf("blocked keyword: %q", kw))
		}
	}

	// Excessive capitalisation in subject
	if excessiveCapsRe.MatchString(subject) {
		result.Score += 2
		result.Reasons = append(result.Reasons, "excessive capitals in subject")
	}

	// Empty subject
	if strings.TrimSpace(subject) == "" {
		result.Score += 1
		result.Reasons = append(result.Reasons, "empty subject")
	}

	// URL analysis
	urls := urlPattern.FindAllString(body, -1)
	if len(urls) > 5 {
		result.Score += 2
		result.Reasons = append(result.Reasons, fmt.Sprintf("excessive URLs (%d)", len(urls)))
	}

	// IP-based URLs (http://1.2.3.4/...)
	if suspiciousURLRe.MatchString(body) {
		result.Score += 3
		result.Reasons = append(result.Reasons, "URL with raw IP address")
	}

	// Homoglyph / mixed-script detection (Cyrillic in Latin text)
	if homoglyphPattern.MatchString(subject) || homoglyphPattern.MatchString(from) {
		result.Score += 3
		result.Reasons = append(result.Reasons, "mixed-script characters (possible homoglyph attack)")
	}

	// HTML-heavy body with little text (common in phishing)
	if len(body) > 200 {
		tagCount := strings.Count(bodyLower, "<")
		textLen := len(strings.TrimSpace(stripHTMLTags(body)))
		if tagCount > 10 && textLen < 100 {
			result.Score += 2
			result.Reasons = append(result.Reasons, "HTML-heavy body with little text content")
		}
	}

	// Determine verdict
	result.IsSpam = result.Score >= spamFilter.Threshold

	return result
}


// --- Public API for admin management ---

// GetSpamFilter returns a copy of the current spam filter config
func GetSpamFilter() *SpamFilter {
	spamMutex.RLock()
	defer spamMutex.RUnlock()

	return &SpamFilter{
		Enabled:          spamFilter.Enabled,
		Threshold:        spamFilter.Threshold,
		BlockedTLDs:      append([]string{}, spamFilter.BlockedTLDs...),
		BlockedKeywords:  append([]string{}, spamFilter.BlockedKeywords...),
		AllowedSenders:   append([]string{}, spamFilter.AllowedSenders...),
		RejectSpam:       spamFilter.RejectSpam,
		AutoBlockDomains: spamFilter.AutoBlockDomains,
	}
}

// SetSpamFilterEnabled toggles the spam filter on/off
func SetSpamFilterEnabled(enabled bool) error {
	spamMutex.Lock()
	defer spamMutex.Unlock()
	spamFilter.Enabled = enabled
	app.Log("mail", "Spam filter enabled=%v", enabled)
	return saveSpamFilter()
}

// SetSpamThreshold updates the spam score threshold
func SetSpamThreshold(threshold int) error {
	if threshold < 1 || threshold > 100 {
		return fmt.Errorf("threshold must be between 1 and 100")
	}
	spamMutex.Lock()
	defer spamMutex.Unlock()
	spamFilter.Threshold = threshold
	app.Log("mail", "Spam threshold set to %d", threshold)
	return saveSpamFilter()
}

// SetRejectSpam sets whether to reject spam at SMTP level
func SetRejectSpam(reject bool) error {
	spamMutex.Lock()
	defer spamMutex.Unlock()
	spamFilter.RejectSpam = reject
	app.Log("mail", "Spam reject mode=%v", reject)
	return saveSpamFilter()
}

// SetAutoBlockDomains sets whether to auto-block spam sender domains
func SetAutoBlockDomains(auto bool) error {
	spamMutex.Lock()
	defer spamMutex.Unlock()
	spamFilter.AutoBlockDomains = auto
	app.Log("mail", "Auto-block spam domains=%v", auto)
	return saveSpamFilter()
}

// AddBlockedTLD adds a TLD to the blocked list
func AddBlockedTLD(tld string) error {
	tld = strings.ToLower(strings.TrimSpace(tld))
	if tld == "" {
		return fmt.Errorf("TLD cannot be empty")
	}
	if !strings.HasPrefix(tld, ".") {
		tld = "." + tld
	}

	spamMutex.Lock()
	defer spamMutex.Unlock()

	for _, existing := range spamFilter.BlockedTLDs {
		if existing == tld {
			return fmt.Errorf("TLD already blocked")
		}
	}

	spamFilter.BlockedTLDs = append(spamFilter.BlockedTLDs, tld)
	app.Log("mail", "Blocked TLD: %s", tld)
	return saveSpamFilter()
}

// RemoveBlockedTLD removes a TLD from the blocked list
func RemoveBlockedTLD(tld string) error {
	tld = strings.ToLower(strings.TrimSpace(tld))
	if !strings.HasPrefix(tld, ".") {
		tld = "." + tld
	}

	spamMutex.Lock()
	defer spamMutex.Unlock()

	for i, existing := range spamFilter.BlockedTLDs {
		if existing == tld {
			spamFilter.BlockedTLDs = append(spamFilter.BlockedTLDs[:i], spamFilter.BlockedTLDs[i+1:]...)
			app.Log("mail", "Unblocked TLD: %s", tld)
			return saveSpamFilter()
		}
	}
	return fmt.Errorf("TLD not found in blocked list")
}

// AddBlockedKeyword adds a keyword to the blocked list
func AddBlockedKeyword(keyword string) error {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return fmt.Errorf("keyword cannot be empty")
	}

	spamMutex.Lock()
	defer spamMutex.Unlock()

	for _, existing := range spamFilter.BlockedKeywords {
		if existing == keyword {
			return fmt.Errorf("keyword already blocked")
		}
	}

	spamFilter.BlockedKeywords = append(spamFilter.BlockedKeywords, keyword)
	app.Log("mail", "Blocked keyword: %s", keyword)
	return saveSpamFilter()
}

// RemoveBlockedKeyword removes a keyword from the blocked list
func RemoveBlockedKeyword(keyword string) error {
	keyword = strings.ToLower(strings.TrimSpace(keyword))

	spamMutex.Lock()
	defer spamMutex.Unlock()

	for i, existing := range spamFilter.BlockedKeywords {
		if existing == keyword {
			spamFilter.BlockedKeywords = append(spamFilter.BlockedKeywords[:i], spamFilter.BlockedKeywords[i+1:]...)
			app.Log("mail", "Removed blocked keyword: %s", keyword)
			return saveSpamFilter()
		}
	}
	return fmt.Errorf("keyword not found in blocked list")
}

// AddAllowedSender adds an email or domain to the allow list
func AddAllowedSender(sender string) error {
	sender = strings.ToLower(strings.TrimSpace(sender))
	if sender == "" {
		return fmt.Errorf("sender cannot be empty")
	}

	spamMutex.Lock()
	defer spamMutex.Unlock()

	for _, existing := range spamFilter.AllowedSenders {
		if existing == sender {
			return fmt.Errorf("sender already allowed")
		}
	}

	spamFilter.AllowedSenders = append(spamFilter.AllowedSenders, sender)
	app.Log("mail", "Allowed sender: %s", sender)
	return saveSpamFilter()
}

// RemoveAllowedSender removes an email or domain from the allow list
func RemoveAllowedSender(sender string) error {
	sender = strings.ToLower(strings.TrimSpace(sender))

	spamMutex.Lock()
	defer spamMutex.Unlock()

	for i, existing := range spamFilter.AllowedSenders {
		if existing == sender {
			spamFilter.AllowedSenders = append(spamFilter.AllowedSenders[:i], spamFilter.AllowedSenders[i+1:]...)
			app.Log("mail", "Removed allowed sender: %s", sender)
			return saveSpamFilter()
		}
	}
	return fmt.Errorf("sender not found in allow list")
}
