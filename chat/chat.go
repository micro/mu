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

	"github.com/micro/mu/app"
	"github.com/micro/mu/data"
)

//go:embed *.json
var f embed.FS

type Prompt struct {
	Rag      []string `json:"rag"`
	Model    string   `json:"model"`
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
<input id="room" name="room" type="hidden">
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
	Summary string `json:"summary"`
}

func Load() {
	// load the feeds file
	b, _ := f.ReadFile("prompts.json")
	if err := json.Unmarshal(b, &prompts); err != nil {
		fmt.Println("Error parsing topics.json", err)
	}

	b, _ = f.ReadFile("rooms.json")
	if err := json.Unmarshal(b, &rooms); err != nil {
		fmt.Println("Error parsing rooms.json", err)
	}

	for topic, _ := range prompts {
		topics = append(topics, topic)
	}

	sort.Strings(topics)

	head = app.Head("chat", topics)

	go loadChats()
}

func loadChats() {
	fmt.Println("Loading rooms", time.Now().String())

	newRooms := map[string]Room{}
	newSummary := ""

	for topic, prompt := range prompts {
		resp, err := askLLM(&Prompt{
			Rag:      []string{prompt},
			Model:    "Fanar",
			Question: "Provide a brief 10 point summary for the topic based on your current knowledge. Only provide the summary, nothing else. Keep it below 512 characters.",
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
		newSummary += fmt.Sprintf(`<div><p><a href="#%s" class="title">%s</a><span class="description">%s</span></p></div>`, topic, topic, desc)
	}

	mutex.Lock()
	rooms = newRooms
	summary = `<div id="summary">` + newSummary + `</div>`
	b, _ := json.Marshal(rooms)
	data.Save("rooms.json", string(b))
	mutex.Unlock()

	time.Sleep(time.Hour)

	go loadChats()
}

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		if ct := r.Header.Get("Content-Type"); ct == "application/json" {
			mutex.RLock()
			defer mutex.RUnlock()

			data := map[string]interface{}{
				"rooms": rooms,
			}
			b, _ := json.Marshal(data)
			w.Write(b)
			return
		}

		mutex.RLock()
		tmpl := app.RenderHTML("Chat", "Chat with AI", fmt.Sprintf(Template, head, summary))
		mutex.RUnlock()

		w.Write([]byte(tmpl))
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
			ctx := r.Form.Get("context")
			msg := r.Form.Get("prompt")
			room := r.Form.Get("room")

			if len(msg) == 0 {
				return
			}
			var ictx interface{}
			json.Unmarshal([]byte(ctx), &ictx)
			data["context"] = ictx
			data["prompt"] = msg
			data["room"] = room
		}

		var context History

		if vals := data["context"]; vals != nil {
			cvals := vals.([]interface{})
			for _, val := range cvals {
				msg := val.(map[string]interface{})
				prompt := fmt.Sprintf("%v", msg["prompt"])
				answer := fmt.Sprintf("%v", msg["answer"])
				context = append(context, Message{Prompt: prompt, Answer: answer})
			}
		}

		q := fmt.Sprintf("%v", data["prompt"])

		var rag []string
		room := fmt.Sprintf("%v", data["room"])
		if len(room) > 0 {
			mutex.RLock()
			room, ok := rooms[room]
			if ok {
				rag = append(rag, room.Summary)
			}
			mutex.RUnlock()
		}

		prompt := &Prompt{
			Rag:      rag,
			Model:    DefaultModel,
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

		output := fmt.Sprintf(Template, head, messages)
		renderHTML := app.RenderHTML("Chat", "Chat with AI", output)

		w.Write([]byte(renderHTML))
	}
}
