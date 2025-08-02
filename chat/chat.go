package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/micro/mu/app"
)

type Prompt struct {
	Model    string
	Context  []string
	Question string
}

var Template = app.RenderHTML("Chat", "Chat with AI", `
<div id="messages"></div>
<form id="chat-form" onsubmit="event.preventDefault(); askLLM(this);">
<input id="context" name="context" type="hidden">
<input id="prompt" name="prompt" type="text" autofocus autocomplete=off>
<button>-></button>
<select name="model" id="model">
  <option value="gpt-4o-mini">gpt-4o-mini</option>
  <option value="gemini-2.5-flash">gemini-2.5-flash</option>
</select>
</form>`)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.Write([]byte(Template))
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
		if vals := data["context"]; vals != nil {
			cvals := vals.([]interface{})
			for _, val := range cvals {
				b, _ := json.Marshal(val)
				ctx = append(ctx, string(b))
			}
		}

		prompt := &Prompt{
			Context:  ctx,
			Question: fmt.Sprintf("%v", data["prompt"]),
		}

		// query the llm
		resp, err := askLLM(context.TODO(), prompt)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		if len(resp) == 0 {
			return
		}

		// save the response
		html := app.Render([]byte(resp))
		data["answer"] = string(html)
		b, _ := json.Marshal(data)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}
}
