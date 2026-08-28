package apps

// Publishing what is in a directory, without the page passing through a model.
//
// # Why this is not apps_create with extra steps
//
// The way to host something built in the shell was to read the file out and
// hand it to Create as an argument, so the page crossed the model twice: once
// written into the box, once quoted back out of it. Against a live instance the
// second crossing is where things came apart — a five-kilobyte page handed to
// apps_create made the model emit its own tool-call delimiters as plain text,
// so nothing was parsed and no app was created, while the same request with a
// forty-seven-byte page worked first time.
//
// This removes that second crossing. It does not remove the first, and saying
// otherwise would be a lie the next person has to discover: a document can only
// get into a box by being written there, and writing it is a tool call carrying
// the whole thing. What is gone is having to say it twice. Half of an
// unreliable thing is not reliability, but it is the half that was redundant,
// and it is the half nothing else can remove.
//
// It also means a hosted app is no longer bounded by what fits in a reply.
//
// # Why it lives in apps
//
// Hosting is what this service owns, and publishing is hosting's verb. The
// alternative — shell growing a Publish that calls apps — would make a generic
// machine know what an app is, which is the wrong direction for a dependency
// between the two to point.
//
// Neither imports the other. Services here are leaves and a test enforces it,
// so the bytes come through internal/workspace: apps asks for a path and never
// learns a container was involved.
//
// # The scanner is the reason this is safe
//
// Everything else that reaches CreateApp came from a model this instance
// prompted. This comes from a directory somebody had a shell in, with the
// network on. ScanApp is what stands between "an agent wrote a file" and "this
// instance serves it on its own domain", so it runs here and refusal is the
// default. A publish that skipped it would be the hole every other door is
// careful not to be.

import (
	"context"
	"fmt"
	"path"
	"strings"

	"mu/internal/quota"
	"mu/internal/service"
	"mu/internal/workspace"
)

// entry is the file a directory is published from. One name, not a guess at
// several, because a directory with two candidates in it has no obvious answer
// and picking one silently is how the wrong page goes live.
const entry = "index.html"

type PublishRequest struct {
	Dir  string `json:"dir" required:"true" description:"The directory on your machine holding the app, relative to /work — e.g. 'tip'. It must contain index.html"`
	Name string `json:"name" description:"What to call the app. Defaults to the directory's name"`
	Slug string `json:"slug" description:"The address to host it at. Defaults to one made from the name. Publishing to a slug you already own replaces that app, which is how you change one"`
}

type PublishResponse struct {
	Slug string `json:"slug" description:"Where it is hosted"`
	Name string `json:"name" description:"What it is called"`
	URL  string `json:"url" description:"The path to open it at"`
	New  bool   `json:"new" description:"True if this created an app, false if it replaced one you already had"`
}

// Publish hosts the index.html in one of the caller's directories.
//
// @example {"dir": "tip", "name": "Tip Calc"}
func (Server) Publish(ctx context.Context, req *PublishRequest, rsp *PublishResponse) error {
	who := service.AccountFrom(ctx)
	if who == "" {
		return fmt.Errorf("sign in to publish an app")
	}
	dir := strings.Trim(strings.TrimSpace(req.Dir), "/")
	if dir == "" {
		return fmt.Errorf("say which directory to publish, e.g. 'tip'")
	}

	// The bytes come from the box, and this is the only place they are read.
	b, err := workspace.ReadFile(ctx, who, path.Join(dir, entry))
	if err != nil {
		return fmt.Errorf("could not read %s: %w", path.Join(dir, entry), err)
	}
	html := string(b)
	if strings.TrimSpace(html) == "" {
		return fmt.Errorf("%s is empty, so there is nothing to host", path.Join(dir, entry))
	}
	if len(html) > MaxHTMLSize {
		return fmt.Errorf("%s is larger than an app may be (%d bytes)", entry, MaxHTMLSize)
	}

	// Refusing is the default. What comes back names what to change, because
	// the caller is usually an agent that can go and change it.
	if issues := ScanApp(html); len(issues) > 0 {
		return fmt.Errorf("%s cannot be hosted as it stands: %s", entry, strings.Join(issues, "; "))
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = dir
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = slugify(name)
	}

	// Publishing again is how an app changes, so a slug the caller already owns
	// is replaced rather than refused. One they do not own is somebody else's
	// app and stays that way — UpdateAppOwned is what enforces that, rather
	// than a check here that could drift from it.
	if existing := GetApp(slug); existing != nil {
		app, err := UpdateAppOwned(who, slug, name, "", "", html, "", -1)
		if err != nil {
			return err
		}
		rsp.Slug, rsp.Name, rsp.URL, rsp.New = app.Slug, app.Name, "/apps/"+app.Slug, false
		return nil
	}

	app, err := CreateApp(who, name, slug, "", "", html, "", 0, false)
	if err != nil {
		return err
	}
	rsp.Slug, rsp.Name, rsp.URL, rsp.New = app.Slug, app.Name, "/apps/"+app.Slug, true
	return nil
}

// publishEndpoint is the Spec entry, kept next to the handler so the two are
// changed together.
var publishEndpoint = service.Endpoint{
	Writes: true,
	Cost:   quota.OpAppCreate,
	Needs:  service.Caller,
	Doc: "Host an app from a directory on your machine. Give it the directory " +
		"under /work holding an index.html — the page is read from there and " +
		"never passes through you, so it can be as large as it needs to be. " +
		"Publishing to a slug you already own replaces that app, which is how " +
		"you change one after editing the file",
}
