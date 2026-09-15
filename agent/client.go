package agent

import (
	"fmt"
	"mu/internal/bookmarks"
	"mu/internal/result"
	"mu/internal/thread"
	"net/http"
)

// ClientMessage is presentation-independent conversation data. No service HTML
// or internal account/transport metadata crosses the client boundary.
type ClientMessage struct {
	ID      string        `json:"id"`
	Role    string        `json:"role"`
	Text    string        `json:"text"`
	Results []result.Item `json:"results,omitempty"`
}

type ClientConversation struct {
	ID              string          `json:"id"`
	Messages        []ClientMessage `json:"messages"`
	Pending         bool            `json:"pending"`
	Agent           string          `json:"agent,omitempty"`
	AgentName       string          `json:"agent_name,omitempty"`
	Attachment      string          `json:"attachment,omitempty"`
	AttachmentTitle string          `json:"attachment_title,omitempty"`
}

// ClientHistory resumes the default web conversation. Explicit old links still
// work, but an ID is never accepted as proof of ownership.
func ClientHistory(account, id string) ClientConversation {
	if id != "" && thread.Get(account, id) == nil {
		id = ""
	}
	if id == "" {
		id = latestThreadFor(account, "", true)
	}
	out := ClientConversation{ID: id, Messages: []ClientMessage{}, Pending: Pending(account, id)}
	if t := thread.Get(account, id); t != nil {
		out.Agent = t.Agent
		out.AgentName = agentTitle(account, t.Agent)
	}
	for _, m := range thread.Messages(account, id, 100) {
		out.Messages = append(out.Messages, ClientMessage{ID: m.ID, Role: m.Role, Text: m.Text, Results: m.Results})
	}
	return out
}

// RememberClientConversation keeps the browser address independent of storage
// identifiers. The cookie selects a thread; ownership is checked on every read.
func RememberClientConversation(w http.ResponseWriter, r *http.Request, id string) {
	http.SetCookie(w, &http.Cookie{Name: "micro_conversation", Value: id, Path: "/", HttpOnly: true, Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https", SameSite: http.SameSiteLaxMode, MaxAge: 2592000})
}
func ClientHistoryForRequest(w http.ResponseWriter, r *http.Request, account string) (ClientConversation, error) {
	id := r.URL.Query().Get("session")
	if id == "" {
		id = r.URL.Query().Get("continue")
	}
	explicit := id != ""
	if explicit {
		id = openThread(account, id)
		if id == "" {
			return ClientConversation{}, fmt.Errorf("Conversation not found")
		}
	}
	var item *bookmarks.Item
	var err error
	attachment := ""
	if !explicit {
		key := r.URL.Query().Get("bookmark")
		if key == "" {
			key = r.URL.Query().Get("saved")
		}
		if key != "" {
			item, err = bookmarks.Get(account, key)
			attachment = "bookmark:" + key
		} else if key = r.URL.Query().Get("item"); key != "" {
			item, err = bookmarks.Source(key)
			attachment = "archive:" + key
		}
		if err != nil {
			return ClientConversation{}, fmt.Errorf("Reading material not found")
		}
	}
	if attachment != "" {
		title := "Reading material"
		if item != nil {
			title = item.Title
		}
		return ClientConversation{Messages: []ClientMessage{}, Attachment: attachment, AttachmentTitle: title}, nil
	}
	if r.URL.Query().Get("new") == "1" {
		return ClientConversation{Messages: []ClientMessage{}}, nil
	}
	if !explicit {
		if c, err := r.Cookie("micro_conversation"); err == nil {
			if t := thread.Get(account, c.Value); t != nil && t.Agent == "" && t.Client == thread.WebClient {
				id = t.ID
			}
		}
	}
	out := ClientHistory(account, id)
	if out.ID != "" && out.Agent == "" {
		RememberClientConversation(w, r, out.ID)
	}
	return out, nil
}
