package apps

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"mu/apps/micro"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/event"
)

// EditMicroApp applies a natural-language change to an existing app and
// re-renders it.
//
// This edits the stored spec rather than regenerating from the description.
// The difference matters: regeneration only sees the new description, so every
// detail the user does not restate is silently dropped — ask for a colour
// change and lose a field. Editing hands the model the current spec and asks
// for it back changed, so anything the instruction does not mention survives.
//
// The previous state is snapshotted first, so any edit is one click from being
// undone at /apps/<slug>/versions.
func EditMicroApp(accountID, slug, instruction string) (*App, error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return nil, fmt.Errorf("say what you want changed")
	}

	mutex.RLock()
	a := apps[slug]
	mutex.RUnlock()
	if a == nil {
		return nil, fmt.Errorf("app not found")
	}
	// Ownership is checked here rather than trusted from the caller, so every
	// path (web form, API, agent tool) enforces it identically.
	if a.AuthorID != accountID {
		return nil, fmt.Errorf("only the author can edit this app")
	}
	if a.Spec == nil {
		return nil, fmt.Errorf("this app was built before edits were supported — fork it to get an editable copy")
	}

	spec, err := micro.Edit(a.Spec, instruction)
	if err != nil {
		return nil, err
	}
	html, err := micro.Render(spec)
	if err != nil {
		return nil, fmt.Errorf("the change did not produce a working app: %w", err)
	}

	mutex.Lock()
	snapshotVersion(a, summarise(instruction))
	a.Spec = spec
	a.HTML = html
	if title := strings.TrimSpace(spec.Title); title != "" {
		a.Name = title
	}
	if icon := emojiSVG(spec.Emoji); icon != "" {
		a.Icon = icon
	}
	a.UpdatedAt = time.Now()
	mutex.Unlock()
	save()

	app.Log("apps", "Edited app %q for %s: %s", slug, accountID, instruction)
	event.Publish(event.Event{Type: "apps_updated"})
	return a, nil
}

// summarise trims an instruction down to a version label.
func summarise(instruction string) string {
	s := strings.Join(strings.Fields(instruction), " ")
	if len(s) > 60 {
		s = s[:57] + "..."
	}
	return s
}

// handleAIEdit serves POST /apps/{slug}/ai-edit. Credits are charged by the
// wallet middleware, which maps this path to app_edit.
func handleAIEdit(w http.ResponseWriter, r *http.Request, slug string) {
	if r.Method != "POST" {
		app.MethodNotAllowed(w, r)
		return
	}
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	instruction := strings.TrimSpace(r.FormValue("instruction"))
	if instruction == "" && app.SendsJSON(r) {
		var req struct {
			Instruction string `json:"instruction"`
		}
		if err := app.DecodeJSON(r, &req); err == nil {
			instruction = strings.TrimSpace(req.Instruction)
		}
	}

	a, err := EditMicroApp(acc.ID, slug, instruction)
	if err != nil {
		app.Error(w, r, http.StatusBadRequest, err.Error())
		return
	}

	if app.WantsJSON(r) || app.SendsJSON(r) {
		app.RespondJSON(w, a)
		return
	}
	http.Redirect(w, r, "/apps/"+a.Slug+"/edit", http.StatusSeeOther)
}
