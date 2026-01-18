package mail

import (
	"encoding/json"
	"sync"
	"time"

	"mu/data"
)

// EmailLog represents a logged email event
type EmailLog struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Direction string    `json:"direction"` // "inbound" or "outbound"
	From      string    `json:"from"`
	To        string    `json:"to"`
	Subject   string    `json:"subject"`
	MessageID string    `json:"message_id,omitempty"`
	Status    string    `json:"status"` // "sent", "received", "failed"
	Error     string    `json:"error,omitempty"`
	Size      int       `json:"size,omitempty"` // bytes
}

var (
	emailLogMutex sync.RWMutex
	emailLogs     []*EmailLog
	maxLogEntries = 500 // Keep last 500 entries
)

func init() {
	loadEmailLog()
}

func loadEmailLog() {
	b, err := data.LoadFile("email_log.json")
	if err != nil {
		return
	}
	emailLogMutex.Lock()
	defer emailLogMutex.Unlock()
	json.Unmarshal(b, &emailLogs)
}

func saveEmailLog() {
	emailLogMutex.RLock()
	defer emailLogMutex.RUnlock()
	b, _ := json.MarshalIndent(emailLogs, "", "  ")
	data.SaveFile("email_log.json", string(b))
}

// LogEmail logs an email event
func LogEmail(direction, from, to, subject, messageID, status, errMsg string, size int) {
	emailLogMutex.Lock()
	defer emailLogMutex.Unlock()

	entry := &EmailLog{
		ID:        generateID(),
		Timestamp: time.Now(),
		Direction: direction,
		From:      from,
		To:        to,
		Subject:   truncate(subject, 100),
		MessageID: messageID,
		Status:    status,
		Error:     errMsg,
		Size:      size,
	}

	emailLogs = append([]*EmailLog{entry}, emailLogs...)

	// Trim to max entries
	if len(emailLogs) > maxLogEntries {
		emailLogs = emailLogs[:maxLogEntries]
	}

	go saveEmailLog()
}

// GetEmailLogs returns recent email logs
func GetEmailLogs(limit int) []*EmailLog {
	emailLogMutex.RLock()
	defer emailLogMutex.RUnlock()

	if limit <= 0 || limit > len(emailLogs) {
		limit = len(emailLogs)
	}

	result := make([]*EmailLog, limit)
	copy(result, emailLogs[:limit])
	return result
}

// GetEmailLogStats returns summary stats
func GetEmailLogStats() map[string]int {
	emailLogMutex.RLock()
	defer emailLogMutex.RUnlock()

	stats := map[string]int{
		"total":          len(emailLogs),
		"inbound":        0,
		"outbound":       0,
		"sent":           0,
		"received":       0,
		"failed":         0,
		"last_24h":       0,
		"last_24h_out":   0,
	}

	cutoff := time.Now().Add(-24 * time.Hour)

	for _, log := range emailLogs {
		if log.Direction == "inbound" {
			stats["inbound"]++
		} else {
			stats["outbound"]++
		}

		switch log.Status {
		case "sent":
			stats["sent"]++
		case "received":
			stats["received"]++
		case "failed":
			stats["failed"]++
		}

		if log.Timestamp.After(cutoff) {
			stats["last_24h"]++
			if log.Direction == "outbound" {
				stats["last_24h_out"]++
			}
		}
	}

	return stats
}

func generateID() string {
	return time.Now().Format("20060102150405.000000")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
