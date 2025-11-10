package chat

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"sync"
	"time"

	"mu/app"
	"mu/data"
)

//go:embed *.json
var f embed.FS

type Prompt struct {
	Rag      []string `json:"rag"`
	Context  History  `json:"context"`
	Question string   `json:"question"`
}

type History []Message

// message history
type Message struct {
	Prompt string
	Answer string
}

var Template = `
<div id="topics">%s</div>
<div id="messages">%s</div>
<form id="chat-form" onsubmit="event.preventDefault(); askLLM(this);">
<input id="context" name="context" type="hidden">
<input id="prompt" name="prompt" type="text" placeholder="Ask a question" autofocus autocomplete=off>
<button>Send</button>
</form>`

var mutex sync.RWMutex

var prompts = map[string]string{}

var rooms = map[string]Room{}

var topics = []string{}

var head string

var summary string

type Room struct {
	Topic   string `json:"topic"`
	Prompt  string `json:"prompt"`
	Summary string `json:"summary,omitempty"`
}

func Load() {
	// load the feeds file
	b, _ := f.ReadFile("prompts.json")
	if err := json.Unmarshal(b, &prompts); err != nil {
		fmt.Println("Error parsing topics.json", err)
	}

	b, _ = data.Load("rooms.json")
	if err := json.Unmarshal(b, &rooms); err != nil {
		fmt.Println("Error parsing rooms.json", err)
	}

	b, _ = data.Load("summary.html")
	summary = string(b)

	for topic, _ := range prompts {
		topics = append(topics, topic)
	}

	sort.Strings(topics)

	//head = app.Head("chat", topics)

	go loadChats()
}

func loadChats() {
	fmt.Println("Loading rooms", time.Now().String())

	newRooms := map[string]Room{}
	newSummary := ""

	for topic, prompt := range prompts {
		// get the index
		res, err := data.Search(prompt, 15, map[string]string{
			"topic": topic,
		})
		if err != nil {
			fmt.Println("Failed to get index for prompt", prompt, err)
			continue
		}

		var rag []string

		for _, val := range res {
			if len(val.Content) > 512 {
				val.Content = val.Content[:512]
			}
			b, _ := json.Marshal(val)
			rag = append(rag, string(b))
		}

		q := fmt.Sprintf(`Provide a 3 bullet points summary for %s based on the context provided.
Do not add any additional information. Do not respond except with the 3 bullets. Keep it below 512 characters but 
ensure none of the content is cutoff. In the event 512 characters is not enough, increase the length as required.
Do not use any additional sources. Order reverse chronologically. Crypto is cryptocurrency. Dev is developers.
Use the url from the json metadata "url" field as the source. In the event its present don't specify anything.
Use the title from the json metadata "title" field if present as the title. In the event its not present create a title. 
Use the description from the json metadata "description" field as the summary. In the event its not present create a summary.
Format each point like so e.g - **title**: summary [source](url)`, topic)

		resp, err := askLLM(&Prompt{
			Rag:      rag,
			Question: q,
		})

		if err != nil {
			fmt.Println("Failed to generate prompt for topic:", topic, err)
			continue
		}
		newRooms[topic] = Room{
			Topic:   topic,
			Prompt:  prompt,
			Summary: resp,
		}

	}

	for _, topic := range topics {
		desc := newRooms[topic].Summary
		if len(desc) == 0 {
			continue
		}
		// render markdown
		desc = string(app.Render([]byte(desc)))
		newSummary += fmt.Sprintf(`<h2>%s</h2><p><span class="description">%s</span></p>`, topic, desc)
	}

	mutex.Lock()
	rooms = newRooms
	summary = `<div id="summary">` + newSummary + `</div>`
	b, _ := json.Marshal(rooms)
	data.Save("rooms.json", string(b))
	data.Save("summary.html", summary)
	mutex.Unlock()

	time.Sleep(time.Hour)

	go loadChats()
}

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		mutex.RLock()
		tmpl := app.RenderHTML("Chat", "Chat with AI", fmt.Sprintf(Template, head, summary))
		mutex.RUnlock()

		w.Write([]byte(tmpl))
		return
	}

	if r.Method == "POST" {
		form := make(map[string]interface{})

		if ct := r.Header.Get("Content-Type"); ct == "application/json" {
			b, _ := ioutil.ReadAll(r.Body)
			if len(b) == 0 {
				return
			}

			json.Unmarshal(b, &form)

			if form["prompt"] == nil {
				return
			}
		} else {
			// save the response
			r.ParseForm()

			// get the message
			ctx := r.Form.Get("context")
			msg := r.Form.Get("prompt")

			if len(msg) == 0 {
				return
			}
			var ictx interface{}
			json.Unmarshal([]byte(ctx), &ictx)
			form["context"] = ictx
			form["prompt"] = msg
		}

		var context History

		if vals := form["context"]; vals != nil {
			cvals := vals.([]interface{})
			for _, val := range cvals {
				msg := val.(map[string]interface{})
				prompt := fmt.Sprintf("%v", msg["prompt"])
				answer := fmt.Sprintf("%v", msg["answer"])
				context = append(context, Message{Prompt: prompt, Answer: answer})
			}
		}

		q := fmt.Sprintf("%v", form["prompt"])

		res, err := data.Search(q, 10, nil)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		var rag []string

		for _, val := range res {
			if len(val.Content) > 512 {
				val.Content = val.Content[:512]
			}
			b, _ := json.Marshal(val)
			rag = append(rag, string(b))
		}

		prompt := &Prompt{
			Rag:      rag,
			Context:  context,
			Question: q,
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
		form["answer"] = string(html)

		// if JSON request then respond with json
		if ct := r.Header.Get("Content-Type"); ct == "application/json" {
			b, _ := json.Marshal(form)
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
			return
		}

		// Format a HTML response
		messages := fmt.Sprintf(`<div class="message"><span class="you">you</span><p>%v</p></div>`, form["prompt"])
		messages += fmt.Sprintf(`<div class="message"><span class="llm">llm</span><p>%v</p></div>`, form["answer"])

		output := fmt.Sprintf(Template, head, messages)
		renderHTML := app.RenderHTML("Chat", "Chat with AI", output)

		w.Write([]byte(renderHTML))
	}
}
