package chat

import (
	"context"
	"net/http"

	"github.com/micro/mu/app"
	"github.com/micro/mu/llm"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// render chat
		res := app.RenderHTML("Chat", "Chat with an LLM", `
<div id="messages"></div>
<form action="/chat" method="POST">
<input id="message" type="message">
<button>-></button>
</form>`)

		w.Write([]byte(res))
		return
	}

	if r.Method == "POST" {
		// save the response
		r.ParseForm()

		// get the message
		msg := r.Form.Get("message")

		// query the llm
		resp := llm.Query(context.TODO(), nil, msg)

		if len(resp) == 0 {
			return
		}

		// save the response
	}
}
