package mail

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"mu/app"
	"mu/auth"
	"mu/data"
)

var mutex sync.RWMutex

// Message represents a mail message
type Message struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Sent      time.Time `json:"sent"`
	Read      bool      `json:"read"`
}

// all messages stored by recipient
var messages = map[string][]*Message{}

// cached html for pages
var html string

var ComposeTemplate = `
<div id="mail">
  <h1>Compose Mail</h1>
  <form action="/mail" method="POST">
    <input type="text" name="to" placeholder="To (username)" required><br>
    <input type="text" name="subject" placeholder="Subject" required><br>
    <textarea name="body" rows="10" placeholder="Message" required style="width: calc(100%% - 60px); padding: 10px; border-radius: 5px; border: 1px solid darkgrey;"></textarea><br>
    <button type="submit">Send</button>
  </form>
  <br>
  <a href="/mail" class="link">Back to Inbox</a>
</div>
`

func init() {
	// load messages from disk
	b, _ := data.Load("messages.json")
	if b != nil {
		json.Unmarshal(b, &messages)
	}
}

// Load initializes the mail system
func Load() {
	fmt.Println("Mail system loaded")
}

// SendMessage sends a message from one user to another
func SendMessage(from, to, subject, body string) error {
	mutex.Lock()
	defer mutex.Unlock()

	// create message
	msg := &Message{
		ID:      fmt.Sprintf("%s-%d", from, time.Now().UnixNano()),
		From:    from,
		To:      to,
		Subject: subject,
		Body:    body,
		Sent:    time.Now(),
		Read:    false,
	}

	// add to recipient's inbox
	if messages[to] == nil {
		messages[to] = []*Message{}
	}
	messages[to] = append(messages[to], msg)

	// save to disk
	data.SaveJSON("messages.json", messages)

	// NOTE: We intentionally do NOT index mail messages for security reasons.
	// Mail is private communication between users and should not be searchable
	// in a shared index that could potentially leak private data.

	return nil
}

// GetInbox retrieves all messages for a user
func GetInbox(username string) []*Message {
	mutex.RLock()
	defer mutex.RUnlock()

	inbox := messages[username]
	if inbox == nil {
		return []*Message{}
	}

	// sort by most recent first
	sorted := make([]*Message, len(inbox))
	copy(sorted, inbox)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Sent.After(sorted[j].Sent)
	})

	return sorted
}

// MarkAsRead marks a message as read
func MarkAsRead(username, messageID string) {
	mutex.Lock()
	defer mutex.Unlock()

	inbox := messages[username]
	for _, msg := range inbox {
		if msg.ID == messageID {
			msg.Read = true
			data.SaveJSON("messages.json", messages)
			break
		}
	}
}

// GetUnreadCount returns the number of unread messages
func GetUnreadCount(username string) int {
	mutex.RLock()
	defer mutex.RUnlock()

	inbox := messages[username]
	count := 0
	for _, msg := range inbox {
		if !msg.Read {
			count++
		}
	}
	return count
}

// LatestMail returns HTML for the latest mail status
func LatestMail(username string) string {
	unread := GetUnreadCount(username)
	if unread > 0 {
		return fmt.Sprintf(`<span style="font-weight: bold;">You've got mail! (%d new)</span>`, unread)
	}
	return "No new messages"
}

// Handler handles mail requests
func Handler(w http.ResponseWriter, r *http.Request) {
	// get the session
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", 401)
		return
	}

	username := sess.Account

	// POST - send a new message
	if r.Method == "POST" {
		r.ParseForm()

		// check if it's a compose or mark as read action
		action := r.Form.Get("action")
		
		if action == "read" {
			messageID := r.Form.Get("id")
			MarkAsRead(username, messageID)
			http.Redirect(w, r, "/mail", 302)
			return
		}

		// it's a send message action
		to := r.Form.Get("to")
		subject := r.Form.Get("subject")
		body := r.Form.Get("body")

		if len(to) == 0 || len(subject) == 0 || len(body) == 0 {
			http.Error(w, "Missing required fields", 400)
			return
		}

		err := SendMessage(username, to, subject, body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		// redirect back to inbox
		http.Redirect(w, r, "/mail", 302)
		return
	}

	// GET - show inbox or compose form
	if r.URL.Query().Get("compose") == "true" {
		html := app.RenderHTML("Compose Mail", "Send a message", ComposeTemplate)
		w.Write([]byte(html))
		return
	}

	// show inbox
	inbox := GetInbox(username)
	
	var content string
	content += `<div id="mail">`
	content += `<h1>Mail</h1>`
	content += fmt.Sprintf(`<a href="/mail?compose=true" class="block"><b>Compose</b></a><br><br>`)
	
	if len(inbox) == 0 {
		content += `<p>No messages in your inbox.</p>`
	} else {
		content += `<div style="max-width: 800px;">`
		for _, msg := range inbox {
			style := ""
			if !msg.Read {
				style = "font-weight: bold;"
			}
			
			content += fmt.Sprintf(`
<div class="card" style="max-width: 100%%; margin-bottom: 15px;">
  <div style="%s">
    <div style="font-size: 0.9em; color: #666;">From: %s</div>
    <div style="font-size: 1.2em; margin: 5px 0;">%s</div>
    <div style="font-size: 0.9em; margin: 10px 0;">%s</div>
    <div style="font-size: 0.8em; color: #666;">%s</div>
    <form action="/mail" method="POST" style="display: inline;">
      <input type="hidden" name="action" value="read">
      <input type="hidden" name="id" value="%s">
      <button type="submit" style="margin-top: 10px;">Mark as Read</button>
    </form>
  </div>
</div>
			`, style, msg.From, msg.Subject, msg.Body, app.TimeAgo(msg.Sent), msg.ID)
		}
		content += `</div>`
	}
	
	content += `</div>`

	html := app.RenderHTML("Mail", "Your inbox", content)
	w.Write([]byte(html))
}
