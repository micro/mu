package app

import (
	"sync"
	"time"
)

const apiLogMaxEntries = 200

// APILogEntry records a single external API call.
type APILogEntry struct {
	Time     time.Time
	Service  string
	Method   string
	URL      string
	Status   int
	Duration time.Duration
	Error    string
}

var (
	apiLogMu      sync.Mutex
	apiLogEntries []*APILogEntry
)

// RecordAPICall appends an external API call record to the in-memory log.
// When the log exceeds apiLogMaxEntries the oldest entry is dropped.
func RecordAPICall(service, method, url string, status int, duration time.Duration, callErr error) {
	entry := &APILogEntry{
		Time:     time.Now(),
		Service:  service,
		Method:   method,
		URL:      url,
		Status:   status,
		Duration: duration,
	}
	if callErr != nil {
		entry.Error = callErr.Error()
	}
	apiLogMu.Lock()
	apiLogEntries = append(apiLogEntries, entry)
	if len(apiLogEntries) > apiLogMaxEntries {
		apiLogEntries = apiLogEntries[len(apiLogEntries)-apiLogMaxEntries:]
	}
	apiLogMu.Unlock()
}

// GetAPILog returns a copy of the API log entries in reverse-chronological order.
func GetAPILog() []*APILogEntry {
	apiLogMu.Lock()
	defer apiLogMu.Unlock()
	result := make([]*APILogEntry, len(apiLogEntries))
	for i, e := range apiLogEntries {
		result[len(apiLogEntries)-1-i] = e
	}
	return result
}
