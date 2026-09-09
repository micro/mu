package app

import (
	"encoding/json"
	"sync"
	"time"

	"mu/internal/data"
)

const apiLogMaxEntries = 500

// APILogEntry records a single external API call.
type APILogEntry struct {
	Kind         string        `json:"kind,omitempty"`
	Model        string        `json:"model,omitempty"`
	RunID        string        `json:"run_id,omitempty"`
	Outcome      string        `json:"outcome,omitempty"`
	Attempt      int           `json:"attempt,omitempty"`
	InputTokens  int           `json:"input_tokens,omitempty"`
	OutputTokens int           `json:"output_tokens,omitempty"`
	Time         time.Time     `json:"time"`
	Service      string        `json:"service"`
	Method       string        `json:"method"`
	URL          string        `json:"url"`
	Status       int           `json:"status"`
	Duration     time.Duration `json:"duration"`
	Error        string        `json:"error,omitempty"`
	RequestBody  string        `json:"-"` // not persisted — too large
	ResponseBody string        `json:"-"` // not persisted — too large
}

var (
	apiLogMu      sync.Mutex
	apiLogEntries []*APILogEntry
	apiLogDirty   bool
)

func init() {
	b, err := data.LoadFile("api_log.json")
	if err == nil && len(b) > 0 {
		json.Unmarshal(b, &apiLogEntries)
	}
	// Start background saver
	go func() {
		for {
			time.Sleep(10 * time.Second)
			apiLogMu.Lock()
			if apiLogDirty {
				data.SaveJSON("api_log.json", apiLogEntries)
				apiLogDirty = false
			}
			apiLogMu.Unlock()
		}
	}()
}

// RecordAPICall appends an external API call record.
// Persisted to disk every 10 seconds, capped at 500 entries.
func RecordAPICall(service, method, url string, status int, duration time.Duration, callErr error, reqBody, respBody string) {
	entry := &APILogEntry{
		Time:         time.Now(),
		Service:      service,
		Method:       method,
		URL:          url,
		Status:       status,
		Duration:     duration,
		RequestBody:  reqBody,
		ResponseBody: respBody,
	}
	if callErr != nil {
		entry.Error = callErr.Error()
	}
	RecordExternalCall(*entry)
}

// RecordExternalCall records metadata without retaining prompts or credentials.
func RecordExternalCall(value APILogEntry) {
	entry := &value
	if entry.Time.IsZero() {
		entry.Time = time.Now()
	}
	apiLogMu.Lock()
	apiLogEntries = append(apiLogEntries, entry)
	if len(apiLogEntries) > apiLogMaxEntries {
		apiLogEntries = apiLogEntries[len(apiLogEntries)-apiLogMaxEntries:]
	}
	apiLogDirty = true
	apiLogMu.Unlock()
}

// APILog returns a copy of the API log entries in reverse-chronological order.
func APILog() []*APILogEntry {
	apiLogMu.Lock()
	defer apiLogMu.Unlock()
	result := make([]*APILogEntry, len(apiLogEntries))
	for i, e := range apiLogEntries {
		result[len(apiLogEntries)-1-i] = e
	}
	return result
}
