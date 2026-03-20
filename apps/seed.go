package apps

import (
	"time"

	"mu/internal/app"

	"github.com/google/uuid"
)

// seedApps creates a set of built-in apps on first run so the directory
// has immediate value. These ship with the platform and are authored by "mu".
func seedApps() {
	seeds := []struct {
		Slug        string
		Name        string
		Description string
		Tags        string
		TemplateID  string
	}{
		{
			Slug:        "timer",
			Name:        "Timer",
			Description: "Countdown timer with start, pause, and reset",
			Tags:        "productivity, timer",
			TemplateID:  "timer",
		},
		{
			Slug:        "calculator",
			Name:        "Calculator",
			Description: "Simple calculator with basic arithmetic operations",
			Tags:        "tools, calculator",
			TemplateID:  "calculator",
		},
		{
			Slug:        "unit-converter",
			Name:        "Unit Converter",
			Description: "Convert between units — temperature, weight, distance",
			Tags:        "tools, converter",
			TemplateID:  "converter",
		},
		{
			Slug:        "flashcards",
			Name:        "Flashcards",
			Description: "Study flashcards — click to flip, arrow keys to navigate",
			Tags:        "education, study",
			TemplateID:  "flashcards",
		},
		{
			Slug:        "notes",
			Name:        "Notes",
			Description: "Quick notes that save automatically",
			Tags:        "productivity, notes",
			TemplateID:  "notes",
		},
		{
			Slug:        "habit-tracker",
			Name:        "Habit Tracker",
			Description: "Track daily habits with a simple counter",
			Tags:        "productivity, habits",
			TemplateID:  "tracker",
		},
	}

	now := time.Now()
	count := 0
	for _, s := range seeds {
		t := GetTemplate(s.TemplateID)
		if t == nil {
			continue
		}

		a := &App{
			ID:          uuid.New().String(),
			Slug:        s.Slug,
			Name:        s.Name,
			Description: s.Description,
			AuthorID:    "mu",
			Author:      "mu",
			HTML:        t.HTML,
			Tags:        s.Tags,
			Public:      true,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		mutex.Lock()
		apps[a.Slug] = a
		mutex.Unlock()
		count++
	}

	if count > 0 {
		save()
		app.Log("apps", "Seeded %d built-in apps", count)
	}
}
