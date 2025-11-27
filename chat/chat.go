package chat

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
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
<div id="topic-selector">
  <div class="topic-tabs">%s</div>
</div>
<div id="messages"></div>
<form id="chat-form" onsubmit="event.preventDefault(); askLLM(this);">
<input id="context" name="context" type="hidden">
<input id="topic" name="topic" type="hidden">
<input id="prompt" name="prompt" type="text" placeholder="Ask a question" autocomplete=off>
<button>Send</button>
</form>`

var mutex sync.RWMutex

var prompts = map[string]string{}

var summaries = map[string]string{}

var topics = []string{}

var head string

func Load() {
	// load the feeds file
	b, _ := f.ReadFile("prompts.json")
	if err := json.Unmarshal(b, &prompts); err != nil {
		fmt.Println("Error parsing topics.json", err)
	}

	for topic, _ := range prompts {
		topics = append(topics, topic)
	}

	sort.Strings(topics)

	head = app.Head("chat", topics)

	go generateSummaries()
}

func generateSummaries() {
	fmt.Println("Generating summaries", time.Now().String())

	newSummaries := map[string]string{}

	for topic, prompt := range prompts {
		// Search for relevant content for each topic
		ragEntries := data.Search(topic, 3)
		var ragContext []string
		for _, entry := range ragEntries {
			contentStr := fmt.Sprintf("%s: %s", entry.Title, entry.Content)
			if len(contentStr) > 500 {
				contentStr = contentStr[:500]
			}
			ragContext = append(ragContext, contentStr)
		}

		resp, err := askLLM(&Prompt{
			Rag:      ragContext,
			Question: prompt,
		})

		if err != nil {
			fmt.Println("Failed to generate summary for topic:", topic, err)
			continue
		}
		newSummaries[topic] = resp
	}

	mutex.Lock()
	summaries = newSummaries
	mutex.Unlock()

	time.Sleep(time.Hour)

	go generateSummaries()
}

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		mutex.RLock()

		// Use Head() to format topics
		topicTabs := app.Head("chat", topics)

		// Pass summaries as JSON to frontend
		summariesJSON, _ := json.Marshal(summaries)

		tmpl := app.RenderHTMLForRequest("Chat", "Chat with AI", fmt.Sprintf(Template, topicTabs), r)
		tmpl = strings.Replace(tmpl, "</body>", fmt.Sprintf(`<script>var summaries = %s;</script></body>`, summariesJSON), 1)

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
			// Keep only the last 5 messages to reduce context size
			startIdx := 0
			if len(cvals) > 5 {
				startIdx = len(cvals) - 5
			}
			for _, val := range cvals[startIdx:] {
				msg := val.(map[string]interface{})
				prompt := fmt.Sprintf("%v", msg["prompt"])
				answer := fmt.Sprintf("%v", msg["answer"])
				context = append(context, Message{Prompt: prompt, Answer: answer})
			}
		}

		q := fmt.Sprintf("%v", form["prompt"])

		// Get topic for enhanced RAG
		topic := ""
		if t := form["topic"]; t != nil {
			topic = fmt.Sprintf("%v", t)
		}

		// Search the index for relevant context (RAG)
		// If topic is provided, use it as additional context for search
		searchQuery := q
		if len(topic) > 0 {
			searchQuery = topic + " " + q
		}
		ragEntries := data.Search(searchQuery, 3)
		var ragContext []string
		for _, entry := range ragEntries {
			// Format each entry as context
			contextStr := fmt.Sprintf("%s: %s", entry.Title, entry.Content)
			if len(contextStr) > 500 {
				contextStr = contextStr[:500]
			}
			if url, ok := entry.Metadata["url"].(string); ok && len(url) > 0 {
				contextStr += fmt.Sprintf(" (Source: %s)", url)
			}
			ragContext = append(ragContext, contextStr)
		}

		prompt := &Prompt{
			Rag:      ragContext,
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
		renderHTML := app.RenderHTMLForRequest("Chat", "Chat with AI", output, r)

		w.Write([]byte(renderHTML))
	}
}
