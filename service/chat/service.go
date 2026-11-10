package chat

import (
	"context"
	"fmt"

	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/types/known/timestamppb"
	"mu/internal/app"
	"mu/internal/service"
	pb "mu/proto/service/chat/v1"
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
type Message = pb.Message

// ── Rooms ───────────────────────────────────────────────────────

type RoomsRequest = pb.RoomsRequest

type RoomsResponse = pb.RoomsResponse

// Rooms lists discussions that currently have activity.
// @example {}
func (Server) Rooms(_ context.Context, req *RoomsRequest, rsp *RoomsResponse) error {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}

	roomsMutex.RLock()
	out := make([]*pb.RoomInfo, 0, len(rooms))
	for _, r := range rooms {
		// Not somebody's conversation. This is the list a tool call gets and the
		// list Card puts on Home, so a pair room reaching it would put "@asim and
		// @henrik" on the front page of everybody's instance. See Listable.
		if !Listable(r.ID) {
			continue
		}
		r.mutex.RLock()
		out = append(out, &pb.RoomInfo{
			Id: r.ID, Type: r.Type, Title: r.Title,
			Participants: int32(len(r.Clients)), LastActivity: timestamppb.New(r.LastActivity),
		})
		r.mutex.RUnlock()
	}
	roomsMutex.RUnlock()

	sort.Slice(out, func(i, j int) bool { return out[i].LastActivity.AsTime().After(out[j].LastActivity.AsTime()) })
	if len(out) > limit {
		out = out[:limit]
	}
	rsp.Rooms = out
	return nil
}

// ── Messages ────────────────────────────────────────────────────

type MessagesRequest = pb.MessagesRequest

type MessagesResponse = pb.MessagesResponse

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

	roomsMutex.RLock()
	room, ok := rooms[id]
	roomsMutex.RUnlock()

	var msgs []RoomMessage
	if ok {
		room.mutex.RLock()
		msgs = append(msgs, room.Messages...)
		room.mutex.RUnlock()
	} else {
		// Not live, but the last messages are persisted.
		msgs = loadRoomMessages(id)
	}

	rsp.Messages = make([]*Message, 0, len(msgs))
	for _, m := range msgs {
		// Arrivals and departures are the room talking about itself. A reader
		// asking for the transcript wants what was said, and an entry with no
		// author in a list whose first field is the author is a line something
		// has to guess about. See RoomMessage.System.
		if m.System {
			continue
		}
		rsp.Messages = append(rsp.Messages, &Message{
			Id: m.ID, Author: m.UserID, Content: m.Content, IsAgent: m.IsLLM, Timestamp: timestamppb.New(m.Timestamp),
		})
	}
	return nil
}

// ── Send ────────────────────────────────────────────────────────

type SendRequest = pb.SendRequest

type SendResponse = pb.SendResponse

// Send posts a message to a room as the caller.
//
// Only into a room that exists: rooms are attached to an item and created when
// somebody opens the discussion on it, so conjuring one from a tool call would
// invent a conversation about nothing. chat_rooms is how you find the id.
//
// It goes through the room's broadcast channel and waits for durable storage,
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
	if text == "" || !utf8.ValidString(text) || len(text) > 8000 {
		return fmt.Errorf("message must contain 1 to 8000 UTF-8 bytes")
	}
	// And not into a conversation you are not in. Reading it was the leak;
	// speaking into it is the other half, and a stranger's line appearing in a
	// private room is worse than a stranger reading one.
	if !Member(id, who) {
		return fmt.Errorf("no room here called %q", id)
	}

	roomsMutex.RLock()
	room, ok := rooms[id]
	roomsMutex.RUnlock()
	if !ok && isGroup(id) {
		room = getOrCreateRoom(id)
		ok = room != nil
	}
	if !ok {
		return fmt.Errorf("no live discussion called %q — chat_rooms lists the ones there are", id)
	}

	if err := post(ctx, room, RoomMessage{UserID: who, Content: text, Timestamp: time.Now()}); err != nil {
		return err
	}
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
	for _, item := range rsp.Rooms {
		r := RoomInfo{ID: item.Id, Type: item.Type, Title: item.Title, Participants: int(item.Participants), LastActivity: item.LastActivity.AsTime()}
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

	// The same row the list on /chat draws, in the same wrapper.
	//
	// It had its own: the name as an inline link with the activity as muted
	// text straight after it, so the second column started wherever the room's
	// name happened to end and eight rooms gave eight different left edges.
	// Reported as janky, and as the activity rather than a description being
	// the thing that would not line up — which is exactly what .room-row
	// already solves on /chat, by pushing the state right the way the inbox
	// pushes a date there.
	//
	// The wrapper is what makes it work: .room-row is styled under .rooms, so
	// a card that borrowed the class without it would get an unstyled span.
	var b strings.Builder
	b.WriteString(`<div class="rooms">`)
	for _, r := range live {
		b.WriteString(roomRow(r.ID, roomName(r.Title), describeRoom(r)))
	}
	b.WriteString(`</div>`)
	return b.String()
}

var Spec = service.Spec{
	Name:        "chat",
	Handler:     new(Server),
	Description: "Chat in live rooms",
	Page:        "/chat",
	Icon:        "chat.png",
	Card:        service.Glance(Card),
	Endpoints: map[string]service.Endpoint{
		"Sources":  {Needs: service.Account, Doc: "List stored message ids owned by the caller"},
		"Source":   {Needs: service.Account, Doc: "Read one stored message owned by the caller, by id"},
		"Messages": {Doc: "Read the recent conversation in a discussion room"},
		"Send":     {Writes: true, Doc: "Say something in a discussion room, as the caller. Use chat_rooms to find the room id", Needs: service.Caller},
		"Rooms":    {Doc: "List discussion rooms that currently have activity"},
	},
}
