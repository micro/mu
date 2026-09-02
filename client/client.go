// Package client is every way a person reaches the agent, and this instance's
// address on each.
//
// # Why this is a package and not a list on a page
//
// Go Micro started with a client and a server: a server registers handlers
// under a name, and a client calls that name. The server grew into the service,
// and this product has those — service.Spec is a name with registered handlers,
// which is the same object it always was.
//
// What it also has, and had never named, is the other side. A person arrives
// over SMS, WhatsApp, mail, the web, the API or the CLI, and each of those is a
// way in with an address, a protocol, and code that receives on it. Those are
// clients. The word was already in the record — internal/thread files every
// conversation under WebClient, SMSClient, WhatsAppClient or ChatClient,
// because which door somebody came through is a fact worth keeping — but there
// was no list, no addresses, and nothing that could be asked "how can this
// instance be reached". Five constants in two packages is a convention, not a
// concept.
//
// The agent is what sits between them: it takes a question from a client and
// calls services by name to answer it. In Go Micro's terms the agent is the
// client — the thing that does the calling — and what this package holds is
// nearer a transport, the shape a request arrives in. That is a good reason not
// to take the analogy further than it goes, and no reason to leave the front
// door of the product unnamed.
//
// # What lives here
//
// The list, and what each one's address is on this instance. Only what works: a
// channel with nothing configured is not offered, because a page that prints a
// number on an instance with no Twilio account sends somebody to text nothing
// and reads as broken rather than unconfigured.
//
// The addresses are why this is a top-level package rather than something under
// internal/. They come from service/sms (the numbers, per channel), service/mail
// (the domain) and the settings (the host), and a service may not import
// another service — so nothing under internal/ or service/ can assemble them.
// A product package may.
//
// The receiving code — agent/sms, agent/mail, agent/chat, the web handler — is
// still where it was. Moving it is the obvious next step and a bigger one; this
// is the name and the list, which is what nothing had.
package client

import (
	"strings"

	"mu/internal/settings"
	"mu/internal/thread"
	"mu/service/mail"
	"mu/service/sms"
)

// A Client is one way in, and this instance's address on it.
type Client struct {
	// ID is the name the record files a conversation under, so what a page
	// offers and what /recall shows you are the same word. Empty for the API,
	// which is a door onto every client rather than one of them — a request
	// arriving there is recorded as whatever it is.
	ID string
	// Label is what it is called out loud.
	Label string
	// Address is where to send something: a number, an address, a URL, a
	// command. The only field somebody actually needs.
	Address string
	// Href is where the label goes when it can be clicked — sms:, mailto:, a
	// path. Empty where the address is not a link.
	Href string
	// Note is the one thing you have to know that the address does not say.
	Note string
}

// All is every way to reach this instance's agent, in the order somebody meets
// them.
func All() []Client {
	var out []Client
	host := Host()

	// The web, always, because you are looking at it. It is the only one that
	// needs no configuration and the only one that cannot be switched off.
	web := Client{ID: thread.WebClient, Label: "Web", Address: host, Href: "/",
		Note: "the box on the front page"}
	if host == "" {
		web.Address = "this page"
	}
	out = append(out, web)

	if n := sms.From(); n != "" && sms.Configured() {
		out = append(out, Client{ID: thread.SMSClient, Label: "SMS", Address: n,
			Href: "sms:" + n, Note: "text it like a person"})
	}
	if w := sms.SendersFor(sms.ChannelWhatsApp); len(w) > 0 && sms.ConfiguredFor(sms.ChannelWhatsApp) {
		out = append(out, Client{ID: thread.WhatsAppClient, Label: "WhatsApp", Address: w[0],
			Href: "https://wa.me/" + strings.TrimPrefix(w[0], "+"),
			Note: "the same conversation, on WhatsApp"})
	}
	// Mail, and only where the domain is a real one.
	//
	// SharedAgentAddress is never empty: mail.ConfiguredDomain falls back to
	// localhost, which is the right answer for an instance talking to itself
	// and the wrong one on a page, where it prints agent@localhost and invites
	// somebody to write to it. That is the exact failure this list exists to
	// avoid, and the only reason it was caught is that a test asked what a bare
	// instance offers.
	if d := strings.TrimSpace(mail.ConfiguredDomain()); d != "" && d != "localhost" {
		if addr := mail.SharedAgentAddress(); addr != "" {
			out = append(out, Client{ID: "mail", Label: "Email", Address: addr,
				Href: "mailto:" + addr, Note: "write to it and it writes back"})
		}
	}
	// The command line, which was forgotten and is a real way in.
	//
	// It has been shippable the whole time — `mu login` then `mu ask` — and it
	// appeared on no page, so the one client with actual documentation was the
	// one nobody was told about. That is the lesson this list exists to stop:
	// a way in that nothing enumerates is a way in that quietly stops being
	// maintained.
	out = append(out, Client{ID: thread.CLIClient, Label: "CLI",
		Address: `mu ask "…"`, Href: "/api",
		Note: "the same agent from a terminal, after mu login"})

	// The API, which is what a program arrives on.
	//
	// Not "curl". curl is one program that speaks HTTP, the way Thunderbird is
	// one program that speaks IMAP — naming the way in after a tool somebody
	// might use would be the only row here that does.
	//
	// The address is /agent/<name> and not /api/v1/…, and that is not a slip.
	// /api/v1/<service>/<method> is the door onto services — the things the
	// agent calls. The agent is not one of them; it is what does the calling,
	// so it has a door of its own. Which is defensible and is also the reason
	// somebody reading a card that says "API" goes to /api and does not find
	// this. Both doors belong on that page.
	if host != "" {
		out = append(out, Client{Label: "API", Address: "POST https://" + host + "/agent/micro",
			Href: "/api", Note: `{"text": "…"} with a token, and read the answer`})
	}
	return out
}

// Host is this instance's own hostname, without a scheme or a trailing slash,
// or "" where the operator has not said what it is.
func Host() string {
	h := strings.TrimSpace(settings.Get("MU_DOMAIN"))
	return strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(h, "https://"), "http://"), "/")
}
