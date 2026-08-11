package sms

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/app"
	"mu/internal/quota"
	"mu/internal/service"
)

// Server is the go-micro handler. Its exported methods become the sms_* tools.
type Server struct{}

func caller(ctx context.Context) (string, error) {
	id := service.AccountFrom(ctx)
	if id == "" {
		return "", fmt.Errorf("sign in to send or read texts")
	}
	return id, nil
}

// ── Send ────────────────────────────────────────────────────────

type SendRequest struct {
	To   string `json:"to" required:"true" description:"The number to text, in international format, e.g. +447700900123. Must be someone in your contacts, a number you have verified as your own, or a number that texted you first"`
	Text string `json:"text" required:"true" description:"What to say. Charged per 160-character segment, so brevity is not only good manners"`
}

type SendResponse struct {
	Message  *Message `json:"message" description:"The message as sent"`
	Segments int      `json:"segments" description:"How many segments it was charged as"`
	Result   string   `json:"result" description:"Confirmation"`
}

// Send texts somebody.
// @example {"to": "+447700900123", "text": "Running ten minutes late"}
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
	rsp.Segments = m.Segments
	rsp.Result = "Sent to " + m.Number + "."
	return nil
}

// ── Inbox ───────────────────────────────────────────────────────

type InboxRequest struct {
	Limit int `json:"limit,omitempty" description:"How many messages to return, newest first (default 50, max 200)"`
}

type InboxResponse struct {
	Messages []Message `json:"messages" description:"Texts sent and received, newest first"`
	Number   string    `json:"number" description:"The number these were sent from and received on"`
}

// Inbox returns the caller's texts, sent and received.
// @example {"limit": 20}
func (Server) Inbox(ctx context.Context, req *InboxRequest, rsp *InboxResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	rsp.Messages = History(owner, req.Limit)
	rsp.Number = From()
	return nil
}

// ── Number ──────────────────────────────────────────────────────

type NumberRequest struct{}

type NumberResponse struct {
	From      string   `json:"from" description:"The number this instance sends from and receives on"`
	Yours     []string `json:"yours" description:"Numbers this account has verified as its own"`
	SentToday int      `json:"sent_today" description:"How many messages this account has sent in the last day"`
	Limit     int      `json:"limit" description:"How many it may send in a day"`
	Countries []string `json:"countries" description:"Country codes this instance will text"`
}

// Number returns the number texts come from, and what this account may still do.
// @example {}
func (Server) Number(ctx context.Context, req *NumberRequest, rsp *NumberResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	rsp.From = From()
	rsp.Yours = Numbers(owner)
	rsp.SentToday = SentToday(owner)
	rsp.Limit = DailyLimit()
	rsp.Countries = allowedCountries()
	return nil
}

// ── Verify ──────────────────────────────────────────────────────

type VerifyRequest struct {
	Number string `json:"number" required:"true" description:"Your own number, in international format"`
	Code   string `json:"code,omitempty" description:"The code that was texted to that number. Omit to have one sent"`
}

type VerifyResponse struct {
	Result   string `json:"result" description:"What happened"`
	Verified bool   `json:"verified" description:"Whether the number is now yours"`
}

// Verify claims a number as the caller's own, in two steps.
//
// Called without a code it texts one to that number; called with the code it
// records the number. Proving it is yours is what makes the first message to it
// legal — otherwise "verify" would be a way to text any stranger once, and the
// rule about only texting numbers you know would be decoration.
// @example {"number": "+447700900123"}
func (Server) Verify(ctx context.Context, req *VerifyRequest, rsp *VerifyResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	number := e164(req.Number)
	if number == "" {
		return fmt.Errorf("%q is not a phone number in international format — use +447700900123", req.Number)
	}
	if code := strings.TrimSpace(req.Code); code != "" {
		if err := Confirm(owner, number, code); err != nil {
			return err
		}
		rsp.Verified, rsp.Result = true, number+" is yours."
		return nil
	}
	if err := StartVerify(owner, number); err != nil {
		return err
	}
	rsp.Result = "Texted a code to " + number + ". Send it back to finish."
	return nil
}

// LoadService registers sms as a service.
func LoadService() {
	if err := service.Register(Spec); err != nil {
		app.Log("sms", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:        "sms",
	Label:       "SMS",
	Handler:     new(Server),
	Description: "Text somebody, and read what they text back",
	Page:        "/sms",
	Icon:        "sms.svg",
	Scoped:      true,
	Endpoints: map[string]service.Endpoint{
		// AccountOnly, not merely priced. Every other paid tool can be reached
		// by an anonymous caller who pays over x402, and that is right for a
		// search: the money covers the cost and nobody else is affected. A text
		// is not like that — an anonymous spammer paying ten pence a message is
		// still a spammer, and what they spend is the number's reputation,
		// which belongs to everybody on this instance and cannot be topped up.
		"Send": {Cost: quota.OpSMSSend, AccountOnly: true, Destructive: true,
			Doc: "Text somebody. Only works for a number already in your contacts, verified as your own, or one that texted you first — an agent cannot text a stranger. Charged per 160-character segment"},
		"Inbox": {AccountOnly: true,
			Doc: "Read the texts this account has sent and received, newest first"},
		"Number": {AccountOnly: true,
			Doc: "The number texts are sent from, which numbers are verified as yours, and how many messages are left today"},
		"Verify": {AccountOnly: true,
			Doc: "Claim a number as your own. Call it with just the number to have a code texted there, then again with the code"},
	},
}
