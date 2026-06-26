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

	content := `<div style="margin-bottom:24px">` + chatComponent(false) + `</div>`

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
