// Package inbox is the agentic inbox: what arrived, and the agent that works
// on it.
//
// A top-level directory because it is a staple by the test in AGENTS.md — a
// user can name it and click it — and because the alternative was where it had
// been living. The inbox surface grew inside agent/, next to the chat, on the
// reasonable-looking grounds that both render conversations. They are not the
// same thing: the chat is a room you talk in, and the inbox is where things
// turn up whether or not you are in the room.
//
// # What this is not
//
// It is not the mail service. service/mail is the MTA — the SMTP server, DKIM,
// the inbound filter — and the message store behind /mail. Nothing here
// duplicates it, and nothing here should: a second mail store is the shape of
// the bug that has cost this repository the most, twice.
//
// It is not the record either. internal/thread holds what was said, on which
// conversation, for which account, written on every turn from every client by
// nobody's decision. This package reads it. Deleting this package loses the
// pages; the record is untouched, which is the test that it is in the right
// layer.
//
// # Where it is going
//
// /inbox is the mailbox: what arrived, whichever channel carried it. The chat
// is /agent/<name>, which is a different thing — a room you talk in, rather
// than where things turn up whether or not you are in it.
//
// # What it may import
//
// internal/, and nothing that consumes tools. It must not import agent/ — an
// inbox that cannot render a conversation without the agent package present is
// an inbox that belongs to the agent. Where it needs something the agent owns,
// the agent hands it over: see Tools, filled in by agent.Load.
package inbox
