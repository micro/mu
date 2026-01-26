package flow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"mu/app"
)

// Flow represents a saved automation
type Flow struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Source    string    `json:"source"`    // The flow syntax
	Schedule  string    `json:"schedule"`  // Cron-like: "every day at 7am", "every hour", etc.
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	LastRun   time.Time `json:"last_run,omitempty"`
	LastError string    `json:"last_error,omitempty"`
	RunCount  int       `json:"run_count"`
}

// Step represents one step in a flow
type Step struct {
	Tool string            `json:"tool"`
	Args map[string]string `json:"args"`
}

// ParsedFlow is the structured representation of a flow
type ParsedFlow struct {
	Trigger string  `json:"trigger"` // "manual", "schedule", "event"
	Cron    string  `json:"cron"`    // If scheduled
	Steps   []*Step `json:"steps"`
}

var (
	flows   = make(map[string]*Flow) // userID -> flowID -> flow
	flowsMu sync.RWMutex
	dataDir string
)

// Load initializes the flow package
func Load() {
	home, _ := os.UserHomeDir()
	dataDir = filepath.Join(home, ".mu", "data", "flows")
	os.MkdirAll(dataDir, 0755)

	// Load all flows
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		app.Log("flow", "Error reading flows dir: %v", err)
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			path := filepath.Join(dataDir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var f Flow
			if err := json.Unmarshal(data, &f); err != nil {
				continue
			}
			flows[f.ID] = &f
		}
	}

	app.Log("flow", "Loaded %d flows", len(flows))

	// Register tools
	registerTools()
}

// Save persists a flow to disk
func (f *Flow) Save() error {
	flowsMu.Lock()
	defer flowsMu.Unlock()

	f.UpdatedAt = time.Now()
	flows[f.ID] = f

	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(dataDir, f.ID+".json")
	return os.WriteFile(path, data, 0644)
}

// Delete removes a flow
func (f *Flow) Delete() error {
	flowsMu.Lock()
	defer flowsMu.Unlock()

	delete(flows, f.ID)
	path := filepath.Join(dataDir, f.ID+".json")
	return os.Remove(path)
}

// Get retrieves a flow by ID
func Get(id string) *Flow {
	flowsMu.RLock()
	defer flowsMu.RUnlock()
	return flows[id]
}

// ListByUser returns all flows for a user
func ListByUser(userID string) []*Flow {
	flowsMu.RLock()
	defer flowsMu.RUnlock()

	var result []*Flow
	for _, f := range flows {
		if f.UserID == userID {
			result = append(result, f)
		}
	}
	return result
}

// Create makes a new flow
func Create(userID, name, source string) (*Flow, error) {
	// Parse to validate
	_, err := Parse(source)
	if err != nil {
		return nil, fmt.Errorf("invalid flow syntax: %v", err)
	}

	f := &Flow{
		ID:        generateID(),
		UserID:    userID,
		Name:      name,
		Source:    source,
		Enabled:   true,
		CreatedAt: time.Now(),
	}

	// Extract schedule from parsed flow
	parsed, _ := Parse(source)
	if parsed.Trigger == "schedule" {
		f.Schedule = parsed.Cron
	}

	if err := f.Save(); err != nil {
		return nil, err
	}

	return f, nil
}

func generateID() string {
	return fmt.Sprintf("flow_%d", time.Now().UnixNano())
}
