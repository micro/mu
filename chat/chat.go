package chat

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/micro/mu/app"
)

type Prompt struct {
	Model    string  `json:"model"`
	Context  History `json:"context"`
	Question string  `json:"question"`
}

type History []Message

// message history
type Message struct {
	Prompt string
	Answer string
}

var Template = app.RenderHTML("Chat", "Chat with AI", `
<div id="messages"></div>
<form id="chat-form" onsubmit="event.preventDefault(); askLLM(this);">
<input id="context" name="context" type="hidden">
<input id="prompt" name="prompt" type="text" placeholder="Ask a question" autofocus autocomplete=off>
<button>Send</button>
<select name="model" id="model">
  <option value="Fanar">Fanar</option>
  <option value="gemini-2.5-flash">Gemini</option>
  <option value="gpt-4o-mini">OpenAI</option>
</select>
</form>`)

var Messages = `
<div id="messages">%s</div>
<form id="chat-form" onsubmit="event.preventDefault(); askLLM(this);">
<input id="context" name="context" type="hidden">
<input id="prompt" name="prompt" type="text" placeholder="Ask a question" autofocus autocomplete=off>
<button>Send</button>
<select name="model" id="model">
  <option value="Fanar">Fanar</option>
  <option value="gemini-2.5-flash">Gemini</option>
  <option value="gpt-4o-mini">OpenAI</option>
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
			ctx := r.Form.Get("context")
			msg := r.Form.Get("prompt")
			model := r.Form.Get("model")

			if len(msg) == 0 {
				return
			}
			if len(model) == 0 {
				model = DefaultModel
			}
			var ictx interface{}
			json.Unmarshal([]byte(ctx), &ictx)
			data["context"] = ictx
			data["prompt"] = msg
			data["model"] = model
		}

		var ctx []Message

		if vals := data["context"]; vals != nil {
			cvals := vals.([]interface{})
			for _, val := range cvals {
				msg := val.(map[string]interface{})
				prompt := fmt.Sprintf("%v", msg["prompt"])
				answer := fmt.Sprintf("%v", msg["answer"])
				ctx = append(ctx, Message{Prompt: prompt, Answer: answer})
			}
		}

		prompt := &Prompt{
			Model:    fmt.Sprintf("%v", data["model"]),
			Context:  ctx,
			Question: fmt.Sprintf("%v", data["prompt"]),
		}

		// query the llm
		resp, err := askLLM(prompt)
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

		// Format a HTML response
		messages := fmt.Sprintf(`<div class="message"><span class="you">you</span><p>%v</p></div>`, data["prompt"])
		messages += fmt.Sprintf(`<div class="message"><span class="llm">llm</span><p>%v</p></div>`, data["answer"])

		output := fmt.Sprintf(Messages, messages)
		renderHTML := app.RenderHTML("Chat", "Chat with AI", output)

		w.Write([]byte(renderHTML))
	}
}
