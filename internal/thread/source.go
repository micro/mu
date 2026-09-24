package thread

// Source points to a service-owned message. Text in a projected message is a
// materialized content, refreshed by Inbox on events. Pages never fetch sources.
type Source struct {
	Service string `json:"service"`
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
}

// InvalidateSource clears cached content after the owning service deletes it.
func InvalidateSource(account, serviceName, id string) {
	ensure()
	mu.Lock()
	defer mu.Unlock()
	for threadID := range owned[account] {
		for _, m := range messages[threadID] {
			if m.Source != nil && m.Source.Service == serviceName && m.Source.ID == id {
				m.Text = "(message deleted)"
				m.SourceHTML = ""
				save()
			}
		}
	}
}
