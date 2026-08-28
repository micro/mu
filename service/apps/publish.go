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
	out, err := PublishDir(ctx, who, req.Dir, req.Name, req.Slug, "")
	if err != nil {
		return err
	}
	*rsp = *out
	return nil
}

// PublishDir is the act itself, for a caller inside this binary that has
// already established who it is acting for — /code, which runs the agent that
// wrote the file and then hosts what it left.
//
// Exported so that publishing from a machine does not have to go out through
// the tool door and back in. The endpoint above is a wrapper on this, so there
// is one implementation and the door adds only identity.
func PublishDir(ctx context.Context, who, reqDir, reqName, reqSlug, because string) (*PublishResponse, error) {
	rsp := &PublishResponse{}
	dir := strings.Trim(strings.TrimSpace(reqDir), "/")
	if dir == "" {
		return nil, fmt.Errorf("say which directory to publish, e.g. 'tip'")
	}

	// The bytes come from the box, and this is the only place they are read.
	b, err := workspace.ReadFile(ctx, who, path.Join(dir, entry))
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", path.Join(dir, entry), err)
	}
	html := string(b)
	if strings.TrimSpace(html) == "" {
		return nil, fmt.Errorf("%s is empty, so there is nothing to host", path.Join(dir, entry))
	}
	if len(html) > MaxHTMLSize {
		return nil, fmt.Errorf("%s is larger than an app may be (%d bytes)", entry, MaxHTMLSize)
	}

	// Refusing is the default. What comes back names what to change, because
	// the caller is usually an agent that can go and change it.
	if issues := ScanApp(html); len(issues) > 0 {
		return nil, fmt.Errorf("%s cannot be hosted as it stands: %s", entry, strings.Join(issues, "; "))
	}

	// A page says what it is in its title, so that is where the name comes from
	// when the caller has not chosen one. The alternative was deriving it from
	// whatever somebody typed, which produced apps called "Page With A Heading
	// That" — the first five words of a sentence are not a name.
	name := strings.TrimSpace(reqName)
	if name == "" {
		name = titleOf(html)
	}
	if name == "" {
		name = dir
	}
	slug := strings.TrimSpace(reqSlug)
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
			return nil, err
		}
		describe(app, because)
		rsp.Slug, rsp.Name, rsp.URL, rsp.New = app.Slug, app.Name, "/apps/"+app.Slug, false
		return rsp, nil
	}

	app, err := CreateApp(who, name, slug, "", "", html, "", 0, false)
	if err != nil {
		return nil, err
	}
	describe(app, because)
	rsp.Slug, rsp.Name, rsp.URL, rsp.New = app.Slug, app.Name, "/apps/"+app.Slug, true
	return rsp, nil
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

// titleOf is the page's own <title>, or empty.
//
// Deliberately not a parser. This runs on a document an agent just wrote, one
// tag is wanted, and pulling in an HTML parse to find it would be a dependency
// bought for a substring. A page with no title, or a silly one, falls back to
// the directory name, which is never wrong — only dull.
func titleOf(html string) string {
	lower := strings.ToLower(html)
	open := strings.Index(lower, "<title")
	if open < 0 {
		return ""
	}
	gt := strings.Index(lower[open:], ">")
	if gt < 0 {
		return ""
	}
	start := open + gt + 1
	end := strings.Index(lower[start:], "</title>")
	if end < 0 {
		return ""
	}
	title := strings.TrimSpace(html[start : start+end])
	// Long enough to be a sentence is not a name.
	if len(title) > 60 {
		return ""
	}
	return title
}

// describe records what a publish was for, on the version it produced.
//
// The version list is the transcript on /code — it is read back rather than
// stored a second time — so a version with no summary is a turn that has
// forgotten what was asked. Every one read "Tip Calculator", the app's own
// name, which is the one thing somebody looking at a list of its versions
// already knows.
//
// The app's description is filled from the first turn for the same reason: it
// is the sentence somebody actually typed, and it beats the name repeated.
func describe(a *App, because string) {
	because = strings.TrimSpace(because)
	if a == nil || because == "" {
		return
	}
	mutex.Lock()
	defer mutex.Unlock()
	if len(a.Versions) > 0 {
		a.Versions[len(a.Versions)-1].Summary = because
	}
	if a.Description == "" || a.Description == a.Name {
		a.Description = because
	}
	save()
}
