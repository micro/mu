package home

// About describes the assistant in product terms.

import (
	"net/http"
	"strings"

	"mu/internal/app"
)

func AboutHandler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString(app.Column())
	b.WriteString(`<div class="page-stack"><p>Micro is a personal AI agent. Ask a question, discuss an idea, or give it something to do.</p><p>Continue the conversation here or through a connected channel. Inbox brings you communications; Work keeps track of tasks, progress and results.</p><p>Micro can use connected tools to search, manage information and carry out requests. You can inspect the outcome and any work that needs your attention.</p></div>`)

	b.WriteString(app.Close())

	app.RespondPublic(w, r, app.Response{
		Title:       "About Micro",
		Description: "Micro is a personal AI agent powered by the open source Mu runtime.",
		HTML:        b.String(),
	})
}
