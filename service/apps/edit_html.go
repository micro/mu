package apps

// Changing an app that was written rather than assembled.
//
// # The hole this fills
//
// There were two kinds of app here and only one of them could be changed.
//
// A micro app has a Spec — a small structured description the renderer turns
// into markup — and EditMicroApp changes the spec and re-renders, which is the
// right way to do it: everything the instruction does not mention survives
// untouched. An app written by writeApp has no spec. It is a document, and a
// document is the whole of it.
//
// EditMicroApp refuses those, in the words a reader actually got: "this app was
// built before edits were supported — fork it to get an editable copy". That
// sentence was true when the only way to get a raw-HTML app was to have made
// one before specs existed. It stopped being true the day writeApp landed, and
// nobody changed the sentence — so the better builder, the one that writes real
// HTML instead of picking one of three shapes, produced apps that the AI editor
// declined to touch and told you to fork. The newest apps on the instance were
// the least editable ones.
//
// # Why a whole document, not a patch
//
// The model is handed the current document and asked for the corrected one
// back. That is expensive — every turn pays for the whole app twice — and it is
// still right for this size of thing. A patch format is a second grammar for
// the model to get wrong, and a patch that fails to apply is a turn that
// produced nothing at all; a whole document that comes back malformed is caught
// by the same scanner and tests as a new one, and gets the same three attempts
// to fix itself. Apps here are capped at 256KB and are usually a few thousand
// tokens. When that stops being true, this is the thing to revisit.
//
// The instruction is repeated in the repair prompt for the same reason the
// build loop repeats the description: a model told only what is broken fixes
// what is broken and forgets what it was for.

import (
	"fmt"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/event"
)

// EditApp applies a change to an app, whichever kind it is.
//
// One door. handleAIEdit called EditMicroApp directly, which is how the raw
// case came to be a message instead of a code path — the dispatch was in the
// caller, so there was nowhere for the second branch to go.
func EditApp(accountID, slug, instruction string) (*App, error) {
	a, err := editable(accountID, slug, instruction)
	if err != nil {
		return nil, err
	}
	if a.Spec != nil {
		return EditMicroApp(accountID, slug, instruction)
	}
	res, err := EditHTMLApp(accountID, slug, instruction)
	if err != nil {
		return nil, err
	}
	return res.App, nil
}

// Edited is what one turn did: the app, and how it went.
//
// Attempts and Problems are the loop made visible. A turn that took three tries
// and a turn that took one are different events, and a page that reports "done"
// for both teaches somebody that the checks are decoration.
type Edited struct {
	App      *App
	Attempts int
	Problems []string
}

// EditHTMLApp rewrites a document app from an instruction.
func EditHTMLApp(accountID, slug, instruction string) (*Edited, error) {
	a, err := editable(accountID, slug, instruction)
	if err != nil {
		return nil, err
	}
	if a.Spec != nil {
		return nil, fmt.Errorf("this app is built from a spec — EditMicroApp changes those")
	}

	mutex.RLock()
	current := a.HTML
	name := a.Name
	mutex.RUnlock()

	instruction = strings.TrimSpace(instruction)
	out, attempts, err := refinement{
		system:    editSystem,
		caller:    "app-edit",
		first:     editQuestion(current, instruction),
		describes: name,
		author:    accountID,
		again: func(problems []string) string {
			return editQuestion(current, instruction) +
				"\n\nYour previous attempt had these problems:\n- " +
				strings.Join(problems, "\n- ") +
				"\n\nReturn the whole corrected document, in the same format."
		},
	}.run()
	if err != nil {
		return nil, err
	}

	mutex.Lock()
	// The previous state first, so any turn is one click from being undone.
	snapshotVersion(a, summarise(instruction))
	a.HTML = out.HTML
	if title := strings.TrimSpace(out.Title); title != "" {
		a.Name = title
	}
	if icon := emojiSVG(out.Emoji); icon != "" {
		a.Icon = icon
	}
	a.UpdatedAt = time.Now()
	mutex.Unlock()
	save()

	app.Log("apps", "Rewrote app %q for %s in %d attempt(s): %s", slug, accountID, attempts, instruction)
	event.Publish(event.Event{Type: "apps_updated"})
	return &Edited{App: a, Attempts: attempts}, nil
}

// editable is the checking every edit does before it costs anything: there is
// an instruction, there is an app, and it is yours.
//
// Ownership is checked here rather than trusted from the caller, so every door
// — the editor form, /code, the API, an agent's tool call — enforces it the
// same way.
func editable(accountID, slug, instruction string) (*App, error) {
	if strings.TrimSpace(instruction) == "" {
		return nil, fmt.Errorf("say what you want changed")
	}
	mutex.RLock()
	a := apps[slug]
	mutex.RUnlock()
	if a == nil {
		return nil, fmt.Errorf("app not found")
	}
	if a.AuthorID != accountID {
		return nil, fmt.Errorf("only the author can edit this app")
	}
	return a, nil
}

// editQuestion is the document and what to do to it.
//
// The document last. A model reading a long prompt weights the end of it
// heavily, and the end here should be the instruction rather than the closing
// tags of something it is about to rewrite — but the instruction also has to be
// read before the document to make sense of it. So it appears at both ends,
// which costs a line and removes the ambiguity.
func editQuestion(current, instruction string) string {
	return "Change this app: " + instruction +
		"\n\nHere is the app as it stands:\n\n" + current +
		"\n\nReturn the whole document with that change applied, and nothing else changed: " +
		instruction
}

const editSystem = buildSystem + `

You are changing an app that already exists, not writing a new one.

- Make the change asked for and nothing else. Every feature, field, style and
  behaviour the instruction does not mention must survive exactly as it is.
- Keep the same title and emoji unless the change is about them.
- Return the complete document, not a fragment and not a description of the
  edit. The whole file replaces the whole file.`
