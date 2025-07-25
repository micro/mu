package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/micro/mu/app"
	"github.com/micro/mu/llm"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// render chat
		res := app.RenderHTML("Chat", "Chat with AI", `
<style>
#chat-form {
	width: 100%;
}
#messages {
	height: calc(100vh - 295px);
	border: 1px solid darkgrey;
	border-radius 5px;
	text-align: left;
	padding: 10px;
	overflow-x: scroll;
}
#prompt {
	width: calc(100% - 40px);
	padding: 10px;
	margin-top: 10px;
}
button {
	padding: 10px;
	margin-top: 10px;
	width: auto;
}
button:hover {
	cursor: pointer;
}
</style>
<div id="messages"></div>
<form id="chat-form" action="/chat" method="POST" onsubmit="event.preventDefault(); askLLM('/chat', this, 'messages');">
<input id="context" name="context" type="hidden">
<input id="prompt" name="prompt" type="text" autofocus autocomplete=off>
<button>-></button>
<script>
document.addEventListener('DOMContentLoaded', onLoad("messages"));
</script>
</form>`)

		w.Write([]byte(res))
		return
	}

	if r.Method == "POST" {
		data := make(map[string]interface{})

		if ct := r.Header.Get("Content-Type"); ct == "application/json" {
			b, _ := ioutil.ReadAll(r.Body)
			if len(b) == 0 {
				return
			}

			json.Unmarshal(b, &data)

			if data["prompt"] == nil {
				return
			}
		} else {
			// save the response
			r.ParseForm()

			// get the message
			msg := r.Form.Get("prompt")
			ctx := r.Form.Get("context")

			if len(msg) == 0 {
				return
			}

			var ictx interface{}
			json.Unmarshal([]byte(ctx), &ictx)
			data["prompt"] = msg
			data["context"] = ictx
		}

		var ctx []string
		if vals := data["context"].([]interface{}); vals != nil {
			for _, val := range vals {
				b, _ := json.Marshal(val)
				ctx = append(ctx, string(b))
			}
		}

		// query the llm
		resp := llm.Query(context.TODO(), ctx, fmt.Sprintf("%v", data["prompt"]))

		if len(resp) == 0 {
			return
		}

		// save the response
		data["answer"] = resp
		b, _ := json.Marshal(data)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}
}
