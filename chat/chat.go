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
  <option value="gemini-2.5-flash">gemini-2.5-flash</option>
  <option value="gpt-4o-mini">gpt-4o-mini</option>
</select>
</form>`)

var Messages = `
<div id="messages">%s</div>
<form id="chat-form" onsubmit="event.preventDefault(); askLLM(this);">
<input id="context" name="context" type="hidden">
<input id="prompt" name="prompt" type="text" autofocus autocomplete=off>
<button>-></button>
<select name="model" id="model">
  <option value="gemini-2.5-flash">gemini-2.5-flash</option>
  <option value="gpt-4o-mini">gpt-4o-mini</option>
</select>
</form>`

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
			if v := data["model"]; v == nil {
				data["model"] = DefaultModel
			}
		} else {
			// save the response
			r.ParseForm()

			// get the message
			msg := r.Form.Get("prompt")
			ctx := r.Form.Get("context")
			model := r.Form.Get("model")

			if len(msg) == 0 {
				return
			}
			if len(model) == 0 {
				model = DefaultModel
			}
			var ictx interface{}
			json.Unmarshal([]byte(ctx), &ictx)
			data["prompt"] = msg
			data["context"] = ictx
			data["model"] = model
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
			Model:    fmt.Sprintf("%v", data["model"]),
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

		// if JSON request then respond with json
		if ct := r.Header.Get("Content-Type"); ct == "application/json" {
			b, _ := json.Marshal(data)
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
			return
		}

		messages := fmt.Sprintf(`<div class="message"><span class="you">you</span><p>%v</p></div>`, data["prompt"])
		messages += fmt.Sprintf(`<div class="message"><span class="llm">llm</span><p>%v</p></div>`, data["answer"])

		output := fmt.Sprintf(Messages, messages)
		renderHTML := app.RenderHTML("Chat", "Chat with AI", output)

		w.Write([]byte(renderHTML))
	}
}
