package api

// A service, rendered at a glance.
//
// Cards were registered per tool, imperatively, from six hand-written lines in
// main.go — api.SetCard("markets_list", "Markets", markets.HTML) and
// five like it. That is the pattern the Spec exists to end: nothing connected a
// card to the service behind it, so a renamed tool lost its card silently and a
// new service could grow one and never be offered it.
//
// A card is a view of a *service*, not of a tool call: markets_list and markets
// are the same picture. So it is declared on the Spec, and everything that
// wants one derives it — the home screen, the agent answering with the markets
// table instead of a paragraph, and /card/<service> for anyone else.
//
// A card is a view, not a widget. It renders and it links; it holds no state
// and takes no input. Anything that does belongs in an app, which already has a
// sandbox and a security boundary — there is no reason to grow a second one.

import (
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/service"
)

// CardForTool renders the card for whatever service a tool belongs to, wrapped
// in the standard container. Empty if that service has no card.
//
// Keyed by tool because the caller is holding a tool result — the agent has
// just run news_list and wants to show the news card rather than describe it.
func CardForTool(name, accountID string) string {
	svc := serviceOf(name)
	if svc == "" {
		return ""
	}
	return wrapCard(service.Label(svc), service.CardFor(svc, accountID).HTML)
}

// CardHandler serves /card/<service> as an HTML fragment, and /card as the list
// of services that have one.
//
// A fragment, not a page: callers embed it. It carries no chrome beyond the
// card container the rest of the app uses.
func CardHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/card"), "/")

	if name == "" {
		out := []map[string]string{}
		for _, s := range service.Cards() {
			out = append(out, map[string]string{
				"service": s.Name, "label": s.NavLabel(),
				"description": s.Description, "url": "/card/" + s.Name, "page": s.Page,
			})
		}
		app.RespondJSON(w, map[string]any{"cards": out})
		return
	}

	// Nothing account-scoped renders a card today — a service holding one
	// person's data shows them a page, not a glance. Guarded anyway: a scoped
	// service growing a card later would otherwise serve one person's view to
	// everybody, and this endpoint is cached.
	if service.AccountScoped(name) {
		app.NotFound(w, r, "No card for that service")
		return
	}

	// The signed-out view, deliberately. This endpoint is a shared fragment
	// with a cache header on it, so it must render the card that is the same
	// for everybody — a personal card served from a shared cache is one
	// account's forecast shown to the next reader. A caller that wants the
	// personal one has an account and can ask the page for it.
	body := wrapCard(service.Label(name), service.CardFor(name, "").HTML)
	if body == "" {
		app.NotFound(w, r, "No card for that service")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Short and shared: a card is the same for everyone, and the renderers
	// behind them refresh on their own schedule.
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.Write([]byte(body))
}

func wrapCard(title, body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	return `<div class="card"><h4>` + html.EscapeString(title) + `</h4>` + body + `</div>`
}
