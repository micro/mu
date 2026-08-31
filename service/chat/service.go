package chat

import (
	"context"
	"fmt"
	htmlpkg "html"
	"sort"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/service"
)

// Server exposes live discussion as a service. Rooms are attached to an item —
// a post, a news story, a video — so the useful questions are "what is being
// discussed", "what was said", and "say this".
//
// Posting was deliberately absent on the argument that a room is a websocket
// conversation between people who are present, so a message from outside that
// session would appear from nobody. That is not right: every message already
// carries the account that sent it, and a call arrives with an account behind
// it exactly like the websocket does. What it produced instead was a service an
// agent could read and not answer — it could see a discussion about a post it
// wrote and had no way to reply. stream and social both post; this is the one
// that could only listen.
type Server struct{}

// RoomInfo describes an active discussion.
type RoomInfo struct {
	ID           string    `json:"id" description:"Room id, e.g. news_456"`
	Type         string    `json:"type" description:"What the room is attached to: post, news or video"`
	Title        string    `json:"title"`
	Participants int       `json:"participants" description:"How many people are connected right now"`
	LastActivity time.Time `json:"last_activity"`
}

// Message is one line of a conversation.
type Message struct {
	Author    string    `json:"author"`
	Content   string    `json:"content"`
	IsAgent   bool      `json:"is_agent" description:"True when the assistant said it"`
	Timestamp time.Time `json:"timestamp"`
}

// ── Rooms ───────────────────────────────────────────────────────

type RoomsRequest struct {
	Limit int `json:"limit,omitempty" description:"How many rooms to return (default 20)"`
}

type RoomsResponse struct {
	Rooms []RoomInfo `json:"rooms" description:"Active discussions, most recently active first"`
}

// Rooms lists discussions that currently have activity.
// @example {}
func (Server) Rooms(_ context.Context, req *RoomsRequest, rsp *RoomsResponse) error {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	mutex.RLock()
	out := make([]RoomInfo, 0, len(rooms))
	for _, r := range rooms {
		// Not somebody's conversation. This is the list a tool call gets and the
		// list Card puts on Home, so a pair room reaching it would put "@asim and
		// @henrik" on the front page of everybody's instance. See Listable.
		if !Listable(r.ID) {
			continue
		}
		r.mutex.RLock()
		out = append(out, RoomInfo{
			ID: r.ID, Type: r.Type, Title: r.Title,
			Participants: len(r.Clients), LastActivity: r.LastActivity,
		})
		r.mutex.RUnlock()
	}
	mutex.RUnlock()

	sort.Slice(out, func(i, j int) bool { return out[i].LastActivity.After(out[j].LastActivity) })
	if len(out) > limit {
		out = out[:limit]
	}
	rsp.Rooms = out
	return nil
}

// ── Messages ────────────────────────────────────────────────────

type MessagesRequest struct {
	Room string `json:"room" description:"Room id, as returned by Rooms"`
}

type MessagesResponse struct {
	Messages []Message `json:"messages" description:"Recent messages, oldest first"`
}

// Messages returns the recent conversation in a room.
// @example {"room": "news_456"}
func (Server) Messages(ctx context.Context, req *MessagesRequest, rsp *MessagesResponse) error {
	id := strings.TrimSpace(req.Room)
	if id == "" {
		return fmt.Errorf("room is required")
	}

	// A private room answers to its members and to nobody else.
	//
	// The web door has checked this since private rooms existed; this one had no
	// check at all, so the whole transcript of a conversation between two people
	// was a tool call away for anybody who could name the room — and the name is
	// two usernames, by construction. Same wording as the page: refusing by name
	// confirms the name.
	if !Member(id, service.AccountFrom(ctx)) {
		return fmt.Errorf("no room here called %q", id)
	}

	mutex.RLock()
	room, ok := rooms[id]
	mutex.RUnlock()

	var msgs []RoomMessage
	if ok {
		room.mutex.RLock()
		msgs = append(msgs, room.Messages...)
		room.mutex.RUnlock()
	} else {
		// Not live, but the last messages are persisted.
		msgs = loadRoomMessages(id)
	}

	rsp.Messages = make([]Message, 0, len(msgs))
	for _, m := range msgs {
		rsp.Messages = append(rsp.Messages, Message{
			Author: m.UserID, Content: m.Content, IsAgent: m.IsLLM, Timestamp: m.Timestamp,
		})
	}
	return nil
}

// ── Send ────────────────────────────────────────────────────────

type SendRequest struct {
	Room string `json:"room" required:"true" description:"Room id, as returned by chat_rooms"`
	// Message, not Content. "Content" is the right word for a document body —
	// files.Content is the file's contents, blog.Content is the post body,
	// web.Content is a page with the navigation stripped out. A chat message is
	// not a document with a body; you send somebody a message, which is what
	// sms.Message already calls it. XMPP names the element <body>, but that is
	// the element inside a <message>: the noun is still the message.
	//
	// Renamed rather than aliased. Required parameters are checked before the
	// handler runs (see missingRequired), so a "content" kept for compatibility
	// would never be reached while "message" is required — an alias that looks
	// like compatibility and is not. A caller on the old name gets an error
	// naming the new one.
	Message string `json:"message" required:"true" description:"What to say"`
}

type SendResponse struct {
	Result string `json:"result"`
}

// Send posts a message to a room as the caller.
//
// Only into a room that exists: rooms are attached to an item and created when
// somebody opens the discussion on it, so conjuring one from a tool call would
// invent a conversation about nothing. chat_rooms is how you find the id.
//
// It goes through the room's broadcast channel rather than appending directly,
// which is what everyone connected sees, what gets persisted, and what moves
// LastActivity — three things that would otherwise each need doing by hand and
// drift apart the first time one of them changed.
// @example {"room":"news_456","message":"Worth reading the primary source on this."}
func (Server) Send(ctx context.Context, req *SendRequest, rsp *SendResponse) error {
	who := service.AccountFrom(ctx)
	if who == "" {
		return fmt.Errorf("sign in to post to a discussion")
	}
	id := strings.TrimSpace(req.Room)
	text := strings.TrimSpace(req.Message)
	if id == "" {
		return fmt.Errorf("room is required — chat_rooms lists them")
	}
	if text == "" {
		return fmt.Errorf("message is required")
	}
	// And not into a conversation you are not in. Reading it was the leak;
	// speaking into it is the other half, and a stranger's line appearing in a
	// private room is worse than a stranger reading one.
	if !Member(id, who) {
		return fmt.Errorf("no room here called %q", id)
	}

	mutex.RLock()
	room, ok := rooms[id]
	mutex.RUnlock()
	if !ok {
		return fmt.Errorf("no live discussion called %q — chat_rooms lists the ones there are", id)
	}

	room.Broadcast <- RoomMessage{UserID: who, Content: text, Timestamp: time.Now()}
	rsp.Result = "sent"
	return nil
}

// LoadService registers chat as a service. Separate from Load, which already
// starts the room machinery.
func LoadService() {
	if err := service.Register(Spec); err != nil {
		app.Log("chat", "service register failed: %v", err)
	}
}

// Card shows what is being discussed. It used to be a question box on the home
// screen that posted to /chat and was answered by a model — a second agent
// input sitting next to the real one. What belongs on a card is the thing only
// this instance knows: who is talking, and about what.
func Card() string {
	var rsp RoomsResponse
	if err := (Server{}).Rooms(context.Background(), &RoomsRequest{Limit: 5}, &rsp); err != nil {
		return ""
	}

	var live []RoomInfo
	for _, r := range rsp.Rooms {
		if r.Participants > 0 || !r.LastActivity.IsZero() {
			live = append(live, r)
		}
	}
	// Nothing live means no card. A card is evidence that a service is doing
	// something; one that says nothing is happening is evidence of the
	// opposite, and it costs a reader the same space as a real one. Home skips
	// a card whose renderer returns empty — that is the contract.
	if len(live) == 0 {
		return ""
	}

	var b strings.Builder
	for _, r := range live {
		b.WriteString(`<div class="row"><a class="link" href="/chat?id=` +
			htmlpkg.EscapeString(r.ID) + `">` + htmlpkg.EscapeString(r.Title) + `</a> `)
		b.WriteString(`<span class="muted">` + describeRoom(r) + `</span></div>`)
	}
	return b.String()
}

var Spec = service.Spec{
	Name:        "chat",
	Handler:     new(Server),
	Description: "Live discussion rooms attached to an item",
	Page:        "/chat",
	Icon:        "chat.png",
	Card:        service.Glance(Card),
	Endpoints: map[string]service.Endpoint{
		"Messages": {Doc: "Read the recent conversation in a discussion room"},
		"Send":     {Writes: true, Doc: "Say something in a discussion room, as the caller. Use chat_rooms to find the room id", Needs: service.Caller},
		"Rooms":    {Doc: "List discussion rooms that currently have activity"},
	},
}
