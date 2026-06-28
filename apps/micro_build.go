package apps

import (
	"fmt"
	htmlpkg "html"
	"net/http"
	"strings"
	"time"

	"mu/apps/micro"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/event"

	"github.com/google/uuid"
)

// BuildMicroApp turns a natural-language description into a saved app using the
// constrained micro-app generator: the model emits a small JSON spec, and a
// deterministic renderer builds a guaranteed-working app. Unlike the old
// free-form builder, there is no model-authored markup to break.
func BuildMicroApp(description, authorID, authorName string) (*App, error) {
	spec, err := micro.Generate(description)
	if err != nil {
		return nil, err
	}
	html, err := micro.Render(spec)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(spec.Title)
	if name == "" {
		name = "App"
	}
	slug := slugify(name)
	if len(slug) < 3 {
		slug = "app-" + slug
	}

	mutex.RLock()
	base := slug
	for i := 2; apps[slug] != nil; i++ {
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	mutex.RUnlock()

	now := time.Now()
	a := &App{
		ID:          uuid.New().String(),
		Slug:        slug,
		Name:        name,
		Description: strings.TrimSpace(description),
		AuthorID:    authorID,
		Author:      authorName,
		Icon:        emojiSVG(spec.Emoji),
		HTML:        html,
		Tags:        spec.Type,
		Public:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	mutex.Lock()
	snapshotVersion(a, "Initial version")
	apps[slug] = a
	mutex.Unlock()
	save()

	app.Log("apps", "Built micro-app %q (%s) for %s", name, spec.Type, authorID)
	event.Publish(event.Event{Type: "apps_updated"})
	return a, nil
}

// emojiSVG wraps an emoji in a minimal SVG so it can serve as an app icon.
// Falls back to the default icon when no emoji is given.
func emojiSVG(emoji string) string {
	emoji = strings.TrimSpace(emoji)
	if emoji == "" {
		return ""
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="32" height="32"><text x="16" y="23" font-size="22" text-anchor="middle">%s</text></svg>`,
		htmlpkg.EscapeString(emoji))
}

// handleMicroGenerate processes a "describe an app" request (POST) — it generates a
// micro-app from the description and redirects to the new app.
func handleMicroGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		app.MethodNotAllowed(w, r)
		return
	}
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	// Gate on chat credits — generation makes one LLM call.
	if QuotaCheck != nil {
		canProceed, _, qerr := QuotaCheck(r, "chat_query")
		if !canProceed {
			msg := "Insufficient credits"
			if qerr != nil {
				msg = qerr.Error()
			}
			app.Error(w, r, http.StatusPaymentRequired, msg)
			return
		}
	}

	description := strings.TrimSpace(r.FormValue("description"))
	if description == "" {
		app.Error(w, r, http.StatusBadRequest, "Describe what you want the app to do")
		return
	}

	a, err := BuildMicroApp(description, acc.ID, acc.Name)
	if err != nil {
		app.Error(w, r, http.StatusBadRequest, "Couldn't build that app: "+err.Error())
		return
	}

	if app.WantsJSON(r) || app.SendsJSON(r) {
		app.RespondJSON(w, a)
		return
	}
	http.Redirect(w, r, "/apps/"+a.Slug, http.StatusSeeOther)
}
