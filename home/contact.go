package home

// How to reach the assistant.
//
// This is one page because there is one question behind five scattered answers.
// The phone number was on /sms, the WhatsApp number on the same page behind a
// pill, the mail address on /agents and in the Connect panel, the curl line on
// /api, and the web box is wherever you already are. Somebody who wanted to know
// "how do I actually use this thing" had to visit four pages and already know
// which four.
//
// # Clients, and where the list of them lives
//
// A client is how you reach the agent, as against a service, which is what the
// agent reaches. internal/thread already names them — WebClient, SMSClient,
// WhatsAppClient, ChatClient, and mail.Client beside its own door — because the
// record has to say which door a conversation came through. What did not exist
// was a *list*: five constants in two packages, and nothing that could
// enumerate them or say what this instance's address is on each.
//
// The addresses are why the list is here and not down beside the constants.
// They come from service/sms (the numbers, per channel), service/mail (the
// domain) and the settings (the host), and a service may not import another
// service — so nothing under internal/ or service/ can assemble them. A page
// package may, which is the same reason the doors row is built here from
// service.Guest.
//
// So the list lives in mu/client, which is the package that names the other
// half of what Go Micro started with: a service is a name with registered
// handlers, and a client is a way to reach one. This page renders it.

import (
	"encoding/json"
	"mu/web"
	"net/http"
	"strings"

	"mu/agent"
	"mu/client"
	"mu/internal/auth"
	"mu/service/sms"
)

// ContactHandler serves the card.
//
// Public, and deliberately. Every address on it is one this instance publishes
// anyway — a number that answers texts, an address that answers mail — and the
// page exists so a stranger can find out what this is before deciding to have
// an account. Gating "how to contact us" behind signing in is the shape of
// question this product is supposed to be the answer to.
func ContactHandler(w http.ResponseWriter, r *http.Request) {
	var acc *auth.Account
	if _, a := auth.TrySession(r); a != nil {
		acc = a
	}
	if web.Page(w, r, "Contact") {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	json.NewEncoder(w).Encode(map[string]any{"channels": client.Personal(), "savable": client.Savable(), "verify_number": acc != nil && !numberVerified(acc.ID)})
}

// VCardHandler serves /contact.vcf — the agent as an address-book entry.
//
// A file rather than a page, and the Content-Type is what does the work: a
// phone offered text/vcard opens its contacts app and asks whether to save.
// Served to anybody, because everything in it is already on the page above it.
func VCardHandler(w http.ResponseWriter, r *http.Request) {
	if !client.Savable() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/vcard; charset=utf-8")
	// The filename a phone shows while it asks. inline rather than attachment:
	// a phone that downloads this to a Files app instead of opening it has put
	// the contact somewhere nobody will look again.
	w.Header().Set("Content-Disposition", `inline; filename="`+vcardName()+`.vcf"`)
	w.Header().Set("Cache-Control", "no-cache, private")
	w.Write([]byte(client.VCard(agent.DefaultName()))) //nolint:errcheck
}

// vcardName is the file's name: the agent's, lowercased, with nothing in it
// that a filesystem would argue about.
func vcardName() string {
	var out strings.Builder
	for _, r := range strings.ToLower(agent.DefaultName()) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			out.WriteRune(r)
		}
	}
	if out.Len() == 0 {
		return "agent"
	}
	return out.String()
}

// numberVerified reports whether this account has proved a number is theirs,
// which is what the phone clients recognise somebody by.
func numberVerified(accountID string) bool {
	return len(sms.Numbers(accountID)) > 0
}

// "" is the card's own, because it is the only thing shaped like this.
//
// A definition list would be the semantic answer and reads badly at this width:
// the label, the address and the note are three columns on a desktop and three
// stacked lines on a phone, which is a grid.
