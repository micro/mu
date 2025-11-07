package apps

import (
	"time"
)

// App represents a mini app with HTML/CSS/JS
type App struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	HTML        string    `json:"html"`
	CSS         string    `json:"css"`
	JS          string    `json:"js"`
	Prompt      string    `json:"prompt,omitempty"`
	Public      bool      `json:"public"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
	UseCount    int       `json:"use_count"`
}
