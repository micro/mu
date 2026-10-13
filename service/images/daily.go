package images

import (
	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/blob"
	"mu/internal/data"
	"mu/internal/settings"
	"strings"
	"time"
)

func dailyLoop() {
	for {
		generateDaily(time.Now().In(imageLocation()))
		time.Sleep(time.Hour)
	}
}

func generateDaily(now time.Time) {
	switch strings.ToLower(strings.TrimSpace(settings.Get("DAILY_IMAGES"))) {
	case "off", "false", "no", "0":
		return
	}
	if !imageDue(now, getDaily()) {
		return
	}
	theme := themeFor(now.YearDay())
	url, err := ai.GenerateImage(theme.prompt)
	if err != nil {
		app.Log("images", "daily generation failed: %v", err)
		return
	}
	d := Daily{URL: url, Prompt: theme.prompt, Theme: theme.name, Date: now.Format("2006-01-02")}
	d.File = storeImage(url, dailyPrefix(d.Date))
	// Keep a successful provider result even if its local download needs retrying.
	if err = data.SaveJSON(dailyKey, d); err != nil {
		app.Log("images", "persist daily image: %v", err)
		return
	}
	dailyMu.Lock()
	daily = d
	dailyMu.Unlock()
	for _, old := range archiveDaily(d) {
		if old.File != "" {
			_ = blob.Delete(old.File)
		}
	}
}
