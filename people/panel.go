// Package people is the layer over everybody else on this instance.
//
// Services, tools, agents, the inbox and Home are all one person's experience —
// your home, your inbox, your agents, your services, every one of them
// singular-possessive. The only shared thing on the instance is the archive,
// and that is content rather than people. Somebody who signs up barely
// encounters anyone.
//
// The parts to fix that already exist and nothing composes them: service/users
// knows everybody here, service/contacts knows who you know, and service/chat
// knows how to talk. Services are leaves; compositions live at top level and
// may import them — inbox/ is a composition over mail, chat, SMS and the
// record. There was no composition over users, contacts and chat, and an empty
// layer is what "there is something missing and I cannot name it" feels like
// from outside.
//
// So: people/, named the way inbox/ and agent/ are — for what it is to a
// person, not for its mechanism.
//
// # Not the inbox, and not a rival to it
//
// The inbox is organised by message: what arrived, what needs you, triage,
// async. This is organised by person: who is here, what you have with them,
// live. Both are communication and neither subsumes the other, which is the
// same distinction as leaving a note and sending a message.
package people

import (
	"mu/internal/app"
	"mu/service/chat"
)

// PanelHTML is the chat, over the page rather than instead of it.
//
// Presence is on Home, so the conversation should be too: seeing that somebody
// is here and then navigating away from the page that told you is the whole
// friction this removes.
//
// # Slides, and does not expand
//
// Expanding pushes the page under it, which means Home relaying out every time
// somebody opens a chat — and Home is a grid, so that is a reflow of
// everything. A panel that slides over is one element moving and nothing else
// changing. The left nav already does exactly this (#nav-container, a
// transform with body.nav-collapsed) and this is that, mirrored, so there is
// one motion in the product rather than two that nearly match.
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
// # It is called Chat, not Lobby
//
// "Lobby" is the room's id showing through. It is the one room with no subject,
// which is a fact about how the chat service is arranged and not something a
// person opening a panel needs to know — and the control that opens this says
// "Open chat", so the heading saying something else made them read as two
// different things.
func PanelHTML() string {
	return `<div id="people-panel" hidden aria-hidden="true">` +
		`<div class="people-panel-head">` +
		`<span class="people-panel-title">Chat</span>` +
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

// OpenLink is the control that opens it.
//
// A button rather than a link, because it does not go anywhere — and the label
// says so. It read "Go to chat" while it was a link to a page; opening a panel
// is a different promise and the word has to match it or the control lies about
// what happens next.
func OpenLink() string {
	return `<button type="button" class="link people-open" onclick="muChatPanel(true)">` +
		`Open chat →</button>`
}

// panelScript opens and closes it, and connects the room once.
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
      // The room machinery, unchanged — see PanelHTML.
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
</script>`
}
