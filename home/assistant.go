package home

import (
	"net/http"

	"mu/internal/app"
)

// AssistantHandler and /home render the same assistant and conversation.
func AssistantHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	Handler(w, r)
}

func assistantNamespace(account string) string {
	if account == "" {
		return "assistant:guest"
	}
	return "assistant:account:" + account
}
