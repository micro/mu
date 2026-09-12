package app

import (
	"mu/internal/auth"
	"mu/internal/service"
)

// ServiceShortcuts is the shared Home and sidebar selection. Empty or stale
// pins fall back to everyday services available on this instance.
func ServiceShortcuts(acc *auth.Account) []service.Spec {
	if acc != nil {
		if pinned := service.Pinned(acc.Pinned); len(pinned) > 0 {
			return pinned
		}
	}
	return service.Pinned([]string{"events", "notes", "docs", "files"})
}
