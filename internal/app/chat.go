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
	SelectionScope  string
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

	return `<div id="mu-search" class="` + wrap + `"><form class="search-bar" id="mu-search-form" method="GET" action="/archive">
    <input id="mu-search-input" type="search" name="q" placeholder="` + htmlpkg.EscapeString(placeholder) + `" maxlength="256"` + focus + `>
    <button type="submit" aria-label="Search">&#x2192;</button>
  </form>` + note + `</div>
`
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
	config, _ := json.Marshal(map[string]any{"contextId": cfg.ContextID, "attachment": cfg.Attachment, "serverOwned": cfg.ServerOwned, "pending": cfg.Pending, "agentName": cfg.AgentName, "storageNS": cfg.StorageNS, "selectionScope": cfg.SelectionScope})
	location := ""
	if cfg.Location {
		location = `<button type="button" id="mu-chat-location" aria-label="Share approximate location" title="Share approximate location"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><circle cx="12" cy="12" r="7"/><circle cx="12" cy="12" r="2"/><path d="M12 2v3m0 14v3M2 12h3m14 0h3"/></svg></button>`
	}
	return `<div id="mu-chat" class="mu-chat-transcript"><div id="mu-chat-conv" role="log" aria-label="Conversation">` + cfg.InitialConvHTML + `</div><form id="mu-chat-form"><textarea id="mu-chat-input" aria-label="Message Micro" placeholder="` + htmlpkg.EscapeString(placeholder) + `" maxlength="1024" rows="1"></textarea>` + location + `<button type="button" id="mu-chat-mic" aria-label="Dictate" title="Dictate using your browser's speech service" hidden><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="9" y="2" width="6" height="12" rx="3"/><path d="M5 10v2a7 7 0 0 0 14 0v-2M12 19v3m-4 0h8"/></svg></button><button type="submit" aria-label="Send">↑</button><span id="mu-chat-voice-status" class="text-muted" role="status"></span></form></div><script type="application/json" id="conversation-config">` + string(config) + `</script><script>` + locationJS + conversationJS + dictationJS + `</script>`
}
