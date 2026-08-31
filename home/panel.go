package home

// The chat, over the page rather than instead of it.
//
// This was package people/, a top-level layer created on the argument that
// services/, tools/, agents/ and the inbox are all one person's experience and
// nothing composed users, contacts and chat. That may still be true. What was
// in the package was this file: the panel on Home and the control that opens
// it, which is a Home component. internal/user is where a person is modelled
// and a second top-level name for the same subject is the thing the naming
// work in this repo keeps having to undo — so it is here, next to the page it
// draws on, until there is a composition to put in a package of its own.
//
// # Slides, and does not expand
//
// Expanding pushes the page under it, which means Home relaying out every time
// somebody opens a chat — and Home is a grid, so that is a reflow of
// everything. A panel that slides over is one element moving and nothing else
// changing. The left nav already does exactly this (#nav-container, a transform
// with body.nav-collapsed) and this is that, mirrored, so there is one motion in
// the product rather than two that nearly match.
//
// # It reuses the room, rather than reimplementing it
//
// bindRoomForm and connectRoomWebSocket are already global in mu.js and bind by
// id — #messages and #chat-form. So the panel carries those ids and the room
// machinery works unchanged: the same socket, the same rendering, the same
// reconnect. A second implementation of a chat client is the thing most likely
// to drift from the first.
//
// Which is also why it is only drawn where there is no room page. On /chat
// those ids already exist and two of each would leave mu.js binding to
// whichever came first.
//
// # It is called Home, not Lobby
//
// "Lobby" was the room's id showing through. It is the one room with no
// subject, which is a fact about how the chat service is arranged and not
// something a person opening a panel needs to know.
//
// "Chat" was the next try and it says the medium, which the reader can see.
// This room is the one attached to Home — it is where the people on this
// instance are, which is what the count above the button has just finished
// saying — so Home is what it is, and the panel and the page it opens over
// agree.

import (
	"html"
	"strings"
	"time"

	"mu/internal/app"
	"mu/service/chat"
)

// panelHTML is the panel itself, hidden until asked for.
func panelHTML() string {
	return `<div id="people-panel" hidden aria-hidden="true">` +
		`<div class="people-panel-head">` +
		`<span class="people-panel-title">Home</span>` +
		`<button type="button" class="people-panel-x" onclick="muChatPanel(false)" ` +
		`aria-label="Close chat">&times;</button></div>` +
		`<div id="messages"></div>` +
		`<form id="chat-form" onsubmit="return false;">` +
		`<input id="topic" name="topic" type="hidden">` +
		`<textarea id="prompt" name="prompt" rows="1" placeholder="Say something" ` +
		`autocomplete="off" onkeydown="if(event.key==='Enter'&&!event.shiftKey)` +
		`{event.preventDefault();this.form.dispatchEvent(new Event('submit'))}"></textarea>` +
		`<button>Send</button></form></div>` + panelScript()
}

// bubbleIcon is a speech bubble, drawn rather than fetched.
const bubbleIcon = `<svg viewBox="0 0 16 16" width="15" height="15" aria-hidden="true">` +
	`<path d="M2.5 3.5h11a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1H6l-3 2.5V11.5h-.5a1 1 0 0 1-1-1v-6a1 1 0 0 1 1-1z" ` +
	`fill="none" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round"/></svg>`

// openChat is the way in: a bubble, and nothing else.
//
// It read "Open chat →" and before that "Go to chat". Both were three words
// where the shape is the word — a speech bubble is the most legible icon in
// software and it does not need to be captioned. The label survives for
// anything not looking at it: aria-label and title both say Open chat.
//
// A button rather than a link, because it does not go anywhere.
func openChat() string {
	return `<button type="button" class="here-chat" onclick="muChatPanel(true)" ` +
		`aria-label="Open chat" title="Open chat">` + bubbleIcon + `</button>`
}

// latest is the last thing said in the room, or nothing.
//
// The window in the door. The panel connects its socket on first open, so
// before you open it nothing on the page knows the room has moved, and a
// bubble on its own is a door with no window: the only way to find out whether
// anything had happened was to open it and look.
//
// So: who, and what. Not a count — "3 new" is a number you still have to open
// the room to understand, and the line itself is the thing that makes somebody
// want to.
//
// And nothing at all when the room is quiet, which is the more important half.
// A room whose last message is from last week is not happening, and a line
// saying so under a live count reads as though it is. An instance where nobody
// has said anything shows a count and a bubble and makes no claim about a
// conversation — which is the honest state for most of them, most of the time.
func latest() string {
	who, text, at := chat.Latest(chat.Lobby)
	if who == "" || text == "" || at.IsZero() || time.Since(at) > quiet {
		return ""
	}
	return `<p class="people-latest"><span class="people-latest-who">@` +
		html.EscapeString(who) + `</span> ` +
		`<span class="people-latest-said">` + html.EscapeString(trimTo(text, 70)) + `</span>` +
		`<span class="people-latest-when">` + html.EscapeString(app.TimeAgo(at)) +
		`</span></p>`
}

// quiet is how old the last message may be and still count as something
// happening. A day: past that it is history, and history under a count of who
// is online reads as if they had just said it.
const quiet = 24 * time.Hour

// trimTo shortens a line to fit under the count. Runes, because cutting a
// multi-byte character in half renders as a replacement glyph.
func trimTo(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// panelScript opens and closes it, connects the room once, and expands the
// list of names.
//
// Once, on first open. Connecting at page load would hold a socket open on
// every visit to Home for a panel nobody asked for; connecting on every open
// would drop and remake it each time somebody glanced at the room.
//
// The escape key closes it, because a thing that covers the page and has no
// keyboard way out is a trap for anybody not using a mouse.
func panelScript() string {
	return `<script>
(function(){
  var panel=document.getElementById('people-panel');
  if(!panel)return;
  var joined=false;
  window.muChatPanel=function(open){
    panel.hidden=!open;
    panel.setAttribute('aria-hidden',String(!open));
    if(!open)return;
    if(!joined){
      joined=true;
      // The room machinery, unchanged — see panelHTML.
      if(window.bindRoomForm)bindRoomForm();
      if(window.connectRoomWebSocket)connectRoomWebSocket(` + app.JSString(chat.Lobby) + `);
    }
    var box=document.getElementById('prompt');
    if(box)box.focus();
  };
  document.addEventListener('keydown',function(e){
    if(e.key==='Escape'&&!panel.hidden)window.muChatPanel(false);
  });
})();
// The count says how many; this says who. Names are the answer to a question
// somebody asked by clicking, which is a different thing from a row of
// strangers you did not.
window.muHereWho=function(btn){
  var list=document.getElementById('here-who');
  if(!list)return;
  list.hidden=!list.hidden;
  btn.setAttribute('aria-expanded',String(!list.hidden));
};
</script>`
}
