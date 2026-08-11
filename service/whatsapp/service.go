package whatsapp

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/quota"
	"mu/internal/service"
	"mu/internal/twilio"
)

// Server is the go-micro handler. Its exported methods become the whatsapp_* tools.
type Server struct{}

func caller(ctx context.Context) (string, error) {
	id := service.AccountFrom(ctx)
	if id == "" {
		return "", fmt.Errorf("sign in to use WhatsApp")
	}
	return id, nil
}

// send is a variable so a test can stand in front of the provider.
var send = deliver

func deliver(to, body string) (string, error) {
	res, err := twilio.Send(url.Values{
		"To": {address(to)}, "From": {address(From())}, "Body": {body},
	})
	if err != nil {
		return "", err
	}
	return res.SID, nil
}

// Send writes to somebody on WhatsApp.
func Send(owner, to, text string) (*Message, error) {
	if owner == "" {
		return nil, fmt.Errorf("sign in to send a WhatsApp message")
	}
	if !Configured() {
		return nil, fmt.Errorf("this instance has no WhatsApp sender configured")
	}

	num := Normalise(to)
	if num == "" {
		return nil, fmt.Errorf("%q is not a phone number in international format — use +447700900123", to)
	}
	if num == From() {
		return nil, fmt.Errorf("that is this instance's own WhatsApp number")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("there is nothing to send")
	}
	if len(text) > maxBody {
		return nil, fmt.Errorf("that is %d characters; a WhatsApp message is %d at most here", len(text), maxBody)
	}

	// Meta's rule, not this instance's: free-form messages are allowed only
	// within twenty-four hours of the other person's last one, and outside that
	// a business may send nothing but a template approved in advance. There are
	// no approved templates here, so the honest shape of it is that you reply to
	// people and do not start conversations.
	if !Open(owner, num) {
		return nil, fmt.Errorf("WhatsApp only allows a message within 24 hours of their last one, "+
			"and %s has not written in that time — they have to message this number first", num)
	}

	// Billed by the conversation rather than the message, so charged the same
	// way: the message that opens a window costs, the rest of the thread does
	// not, because we are not billed for it either.
	fresh := Fresh(owner, num)
	if fresh && quota.Metered(quota.OpWhatsAppSend) {
		ok, _, cost, err := quota.CheckQuota(owner, quota.OpWhatsAppSend)
		if err != nil {
			return nil, err
		}
		if !ok || quota.BalanceOf(owner) < cost {
			return nil, fmt.Errorf("starting a WhatsApp conversation costs %d credits and there are not enough on this account", cost)
		}
	}

	if _, err := send(num, text); err != nil {
		return nil, err
	}
	if fresh {
		if err := quota.ConsumeWith(owner, quota.OpWhatsAppSend, map[string]interface{}{"to": num}); err != nil {
			app.Log("whatsapp", "charging %s for a sent message: %v", owner, err)
		}
	}
	return Record(owner, "out", num, text), nil
}

// Normalise is the package's own name for a phone number in one spelling.
func Normalise(s string) string { return number("whatsapp:" + s) }

// ── Send ────────────────────────────────────────────────────────

type SendRequest struct {
	To   string `json:"to" required:"true" description:"Their number in international format, e.g. +447700900123. They must have messaged this instance within the last 24 hours"`
	Text string `json:"text" required:"true" description:"What to say"`
}

type SendResponse struct {
	Message *Message `json:"message" description:"The message as sent"`
	Result  string   `json:"result" description:"Confirmation"`
}

// Send writes to somebody on WhatsApp.
// @example {"to": "+447700900123", "text": "On my way"}
func (Server) Send(ctx context.Context, req *SendRequest, rsp *SendResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	m, err := Send(owner, req.To, req.Text)
	if err != nil {
		return err
	}
	rsp.Message = m
	rsp.Result = "Sent to " + m.Number + " on WhatsApp."
	return nil
}

// ── History ─────────────────────────────────────────────────────

type HistoryRequest struct {
	Limit int `json:"limit,omitempty" description:"How many messages to return, newest first (default 50, max 200)"`
}

type HistoryResponse struct {
	Messages []Message `json:"messages" description:"Messages sent and received, newest first"`
	Number   string    `json:"number" description:"The WhatsApp number these went through"`
}

// History returns the caller's WhatsApp messages, sent and received.
// @example {"limit": 20}
func (Server) History(ctx context.Context, req *HistoryRequest, rsp *HistoryResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	rsp.Messages = History(owner, req.Limit)
	rsp.Number = From()
	return nil
}

// ── Open ────────────────────────────────────────────────────────

type OpenRequest struct {
	Number string `json:"number,omitempty" description:"One number to ask about. Omit for everyone you can currently write to"`
}

type Conversation struct {
	Number string    `json:"number" description:"Their number"`
	Until  time.Time `json:"until" description:"When the window closes and they have to write again first"`
}

type OpenResponse struct {
	Conversations []Conversation `json:"conversations" description:"Who can be written to right now, and until when"`
	Note          string         `json:"note" description:"Why the list is what it is"`
}

// Open lists who this account may write to on WhatsApp right now.
//
// Worth a tool of its own because the answer changes on its own: WhatsApp only
// allows a free-form message within 24 hours of the other person's last one, so
// an agent that means to write later needs to know whether it still can, and the
// only alternative is to find out by being refused.
// @example {}
func (Server) Open(ctx context.Context, req *OpenRequest, rsp *OpenResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	rsp.Note = "WhatsApp allows a message only within 24 hours of their last one. " +
		"Anyone not listed has to write first."

	seen := map[string]bool{}
	for _, m := range History(owner, 200) {
		if m.Direction != "in" || seen[m.Number] {
			continue
		}
		seen[m.Number] = true
		if req.Number != "" && Normalise(req.Number) != m.Number {
			continue
		}
		if until := OpenUntil(owner, m.Number); !until.IsZero() && time.Now().Before(until) {
			rsp.Conversations = append(rsp.Conversations, Conversation{Number: m.Number, Until: until})
		}
	}
	return nil
}

// LoadService registers whatsapp as a service.
func LoadService() {
	if err := service.Register(Spec); err != nil {
		app.Log("whatsapp", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:        "whatsapp",
	Label:       "WhatsApp",
	Handler:     new(Server),
	Description: "Reply to people on WhatsApp, and read what they send",
	Page:        "/whatsapp",
	Icon:        "whatsapp.svg",
	Scoped:      true,
	Endpoints: map[string]service.Endpoint{
		// Account-only for the same reason texts are: what an anonymous sender
		// spends is the sender's standing with Meta, and a WhatsApp number that
		// gets reported enough is not one a payment can restore.
		"Send": {Cost: quota.OpWhatsAppSend, AccountOnly: true, Destructive: true,
			Doc: "Send a WhatsApp message. Only to somebody who has written to this instance in the last 24 hours — WhatsApp's own rule, not this one, and there is no way around it without templates approved in advance"},
		"History": {AccountOnly: true,
			Doc: "Read the WhatsApp messages this account has sent and received, newest first"},
		"Open": {AccountOnly: true,
			Doc: "List who can be written to on WhatsApp right now and until when. Check this before promising to follow up later"},
	},
}
