package app

import (
	_ "embed"
	"encoding/json"
	htmlpkg "html"
)

//go:embed location.js
var locationJS string

func JSString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

func JSAttr(s string) string {
	return htmlpkg.EscapeString(JSString(s))
}

type ChatConfig struct {
	Ask             bool
	Location        bool
	ContextID       string
	Attachment      string
	InitialConvHTML string
	AgentName       string
	Placeholder     string
	StorageNS       string
	ServerOwned     bool
	Pending         bool
}

var AgentReady func() bool

type SearchBoxOpts struct {
	Why     string
	Centred bool
	Focused bool
}

func SearchBox(o SearchBoxOpts) string { return searchBox(o) }

func askAction() bool { return AgentReady == nil || AgentReady() }

func AgentIsReady() bool { return askAction() }

func searchBox(o SearchBoxOpts) string {
	note := ""
	if o.Why != "" {
		note = `<p class="mu-search-why">` + o.Why +
			TextLink("/admin/config", "/admin/config") + `.</p>`
	}

	placeholder := "Search everything here"

	wrap := "mu-search"
	if o.Centred {
		wrap += " mu-search-mid"
	}
	focus := ""
	if o.Focused {
		focus = " autofocus"
	}

	return `<div id="mu-search" class="` + wrap + `"><form id="mu-search-form" method="GET" action="/archive">
    <input id="mu-search-input" type="search" name="q" placeholder="` + htmlpkg.EscapeString(placeholder) + `" maxlength="256"` + focus + `>
    <button type="submit" aria-label="Search">&#x2192;</button>
  </form>` + note + `</div>
<style>
/* Left, not centred. The rest of the page it sits on starts at the
   left margin, and a box centred inside a left-aligned column reads as
   misaligned rather than as centred. That includes the front page, which used
   to be the exception and is not any more — everything on it hangs off one
   edge. Nothing passes Centred today; it is kept because a page with a box and
   nothing else is a shape somebody will want again. */
#mu-search{max-width:760px;margin:0;width:100%}
#mu-search.mu-search-mid{max-width:560px;margin:0 auto}
#mu-search-form{display:flex;align-items:center;gap:0;border:1px solid var(--card-border,#ddd);
  border-radius:6px;background:var(--card-background,#fff);padding:4px 4px 4px 12px;transition:border-color .2s}
#mu-search-form:focus-within{border-color:#999}
#mu-search-input{flex:1;border:0;outline:0;font:inherit;font-size:16px;padding:8px 0;background:transparent;
  color:var(--text-primary,#111);min-width:0}
#mu-search-form button{flex:none;border:0;border-radius:4px;background:var(--btn-primary,#111);color:#fff;
  font:inherit;width:32px;height:32px;cursor:pointer}
#mu-search-form button:hover{background:var(--btn-primary-hover,#333)}
.mu-search-why{max-width:760px;margin:8px 0 0;color:var(--text-muted,#888);font-size:13px;line-height:1.6}
</style>`
}

//go:embed conversation.js
var conversationJS string

//go:embed conversation.css
var conversationCSS string

//go:embed dictation.js
var dictationJS string

// ChatComponent is the single conversation renderer for guests and accounts.
func ChatComponent(cfg ChatConfig) string {
	if !cfg.Ask {
		return searchBox(SearchBoxOpts{})
	}
	if !AgentIsReady() {
		return searchBox(SearchBoxOpts{Why: "No model is configured, so the agent cannot answer yet. Add a provider at "})
	}
	placeholder := cfg.Placeholder
	if placeholder == "" {
		placeholder = "What do you need?"
	}
	config, _ := json.Marshal(map[string]any{"contextId": cfg.ContextID, "attachment": cfg.Attachment, "serverOwned": cfg.ServerOwned, "pending": cfg.Pending, "agentName": cfg.AgentName, "storageNS": cfg.StorageNS})
	location := ""
	if cfg.Location {
		location = `<button type="button" id="mu-chat-location" aria-label="Share approximate location" title="Share approximate location"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><circle cx="12" cy="12" r="7"/><circle cx="12" cy="12" r="2"/><path d="M12 2v3m0 14v3M2 12h3m14 0h3"/></svg></button>`
	}
	return `<style>` + conversationCSS + `</style><div id="mu-chat" class="mu-chat-transcript"><div id="mu-chat-conv" role="log" aria-label="Conversation">` + cfg.InitialConvHTML + `</div><form id="mu-chat-form"><textarea id="mu-chat-input" aria-label="Message Micro" placeholder="` + htmlpkg.EscapeString(placeholder) + `" maxlength="1024" rows="1"></textarea>` + location + `<button type="button" id="mu-chat-mic" aria-label="Dictate" title="Dictate using your browser's speech service" hidden><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="9" y="2" width="6" height="12" rx="3"/><path d="M5 10v2a7 7 0 0 0 14 0v-2M12 19v3m-4 0h8"/></svg></button><button type="submit" aria-label="Send">↑</button><span id="mu-chat-voice-status" class="text-muted" role="status"></span></form></div><script type="application/json" id="conversation-config">` + string(config) + `</script><script>` + locationJS + conversationJS + dictationJS + `</script>`
}
