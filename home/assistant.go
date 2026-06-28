package home

import (
	"net/http"

	"mu/internal/app"
)

func AssistantHandler(w http.ResponseWriter, r *http.Request) {
	prefill := r.URL.Query().Get("q")
	if prefill == "" {
		prefill = r.URL.Query().Get("prompt")
	}

	// The command box on top, then the always-on overview below — `/` is a home
	// (a live summary you can act on), not a blank chat. The overview hides as
	// soon as you start a conversation (handled in the chat component). The
	// overview shares the chat's centered column so input and cards align, and
	// we drop the chrome's page title on this surface.
	content := `<style>#page-title{display:none}</style>`
	content += `<div style="margin-bottom:24px">` + chatComponent(false) + `</div>`
	content += `<div id="mu-overview" style="max-width:750px;margin:0 auto">` + HomeOverviewHTML() + `</div>`

	// Auto-submit if a query param was provided (e.g. from a deep link).
	if prefill != "" {
		content += `<script>(function(){
var input=document.getElementById('mu-chat-input');
if(input&&window.muChatAsk){input.value=` + jsString(prefill) + `;window.muChatAsk(input.value);}
history.replaceState(null,'','/');
})()</script>`
	}

	html := app.RenderHTMLForRequest("Micro", "Your personal AI agent", content, r)
	w.Write([]byte(html))
}
