package apps

// Making an app, rather than finding one.
//
// Create, Edit, Fork, Run and Test were the last apps tools written out by
// hand: apps_create pointed at POST /apps/new and went through the mux to reach
// the page's form handler, and the other four were closures in the assembly
// calling functions this package already exported. Either way the apps Spec
// said the service could be searched and read, and said nothing about the half
// where an app comes from.
//
// CreateApp is new; the rest call what was already here. It was inside the form
// handler, so the one path that could make an app was the one that had parsed a
// form — which is why apps_create had to arrive as a fake HTTP request to use
// it.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/origin"
	"mu/internal/service"

	"github.com/google/uuid"
)

// CreateApp saves a new app owned by author. It is the whole of what creating
// an app is, extracted from the form handler so that a caller who is not a form
// can do it.
func CreateApp(authorID, name, slug, description, tags, html, icon string, price int, public bool) (*App, error) {
	acc, err := auth.GetAccount(authorID)
	if err != nil {
		return nil, fmt.Errorf("account not found")
	}
	if slug == "" && name != "" {
		slug = slugify(name)
	}
	if description == "" {
		description = name
	}
	if name == "" || slug == "" || html == "" {
		return nil, fmt.Errorf("name and html are required")
	}
	if !slugRe.MatchString(slug) {
		return nil, fmt.Errorf("slug must be 3-50 characters: lowercase letters, numbers and hyphens")
	}
	if len(html) > MaxHTMLSize {
		return nil, fmt.Errorf("html exceeds the 256KB limit")
	}
	if price < 0 {
		price = 0
	}
	if price > 1000 {
		price = 1000
	}

	// A taken slug gets a number rather than an error: the caller asked for a
	// name, not for that exact URL, and refusing would make an agent guess.
	mutex.RLock()
	base := slug
	for i := 2; apps[slug] != nil; i++ {
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	mutex.RUnlock()

	now := time.Now()
	a := &App{
		ID: uuid.New().String(), Slug: slug, Name: name,
		Description: description, AuthorID: acc.ID, Author: acc.Name,
		Icon: icon, HTML: html, Tags: tags, Price: price, Public: public,
		CreatedAt: now, UpdatedAt: now,
	}
	mutex.Lock()
	snapshotVersion(a, "Initial version")
	apps[slug] = a
	mutex.Unlock()
	save()
	app.Log("apps", "Created app %q by %s", name, acc.ID)
	return a, nil
}

func author(ctx context.Context) (string, error) {
	id := service.AccountFrom(ctx)
	if id == "" {
		return "", fmt.Errorf("sign in to author apps")
	}
	return id, nil
}

// ── Create ──────────────────────────────────────────────────────

type CreateRequest struct {
	Name        string `json:"name" required:"true" description:"App name, e.g. \"Pomodoro Timer\""`
	HTML        string `json:"html" required:"true" description:"The app's HTML, inline CSS and JavaScript included, up to 256KB"`
	Slug        string `json:"slug" description:"URL-friendly id, e.g. pomodoro-timer. Derived from the name if omitted"`
	Description string `json:"description" description:"What the app does. Defaults to the name"`
	Tags        string `json:"tags" description:"Comma-separated tags"`
	Icon        string `json:"icon" description:"An SVG icon"`
	Price       int    `json:"price" description:"Credits charged per use, 0 for free, up to 1000"`
}

type CreateResponse struct {
	Result string `json:"result" description:"Confirmation, with the app's slug and URL"`
}

// Create saves a new app: a small, self-contained HTML tool hosted here.
// @example {"name": "Pomodoro Timer", "html": "<h1>…</h1>"}
func (Server) Create(ctx context.Context, req *CreateRequest, rsp *CreateResponse) error {
	who, err := author(ctx)
	if err != nil {
		return err
	}
	a, err := CreateApp(who, req.Name, req.Slug, req.Description, req.Tags, req.HTML, req.Icon, req.Price, true)
	if err != nil {
		return err
	}
	rsp.Result = fmt.Sprintf("Created %s at /apps/%s.", a.Name, a.Slug)
	return nil
}

// ── Edit ────────────────────────────────────────────────────────

type EditRequest struct {
	Slug        string `json:"slug" required:"true" description:"The app's URL slug, e.g. pomodoro-timer"`
	Name        string `json:"name" description:"New name. Left alone if omitted"`
	Description string `json:"description" description:"New description. Left alone if omitted"`
	Tags        string `json:"tags" description:"New comma-separated tags. Left alone if omitted"`
	HTML        string `json:"html" description:"New HTML, up to 256KB. Left alone if omitted"`
	Icon        string `json:"icon" description:"New SVG icon. Left alone if omitted"`
	Price       int    `json:"price" description:"Credits charged per use, 0 for free, up to 1000"`
}

type EditResponse struct {
	Result string `json:"result" description:"Confirmation"`
}

// Edit updates an app the caller owns. Fields left out keep their value.
// @example {"slug": "pomodoro-timer", "description": "A 25-minute timer"}
func (Server) Edit(ctx context.Context, req *EditRequest, rsp *EditResponse) error {
	who, err := author(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(req.Slug) == "" {
		return fmt.Errorf("slug is required")
	}
	a, err := UpdateAppOwned(who, req.Slug, req.Name, req.Description, req.Tags, req.HTML, req.Icon, req.Price)
	if err != nil {
		return err
	}
	rsp.Result = fmt.Sprintf("Updated %s.", a.Name)
	return nil
}

// ── Fork ────────────────────────────────────────────────────────

type ForkRequest struct {
	Slug    string `json:"slug" required:"true" description:"Slug of the app to fork"`
	NewSlug string `json:"new_slug" description:"Slug for the copy. Generated from the original if omitted"`
}

type ForkResponse struct {
	Result string `json:"result" description:"Confirmation, with the new app's slug and URL"`
}

// Fork copies an app into the caller's account, to change independently.
// @example {"slug": "pomodoro-timer"}
func (Server) Fork(ctx context.Context, req *ForkRequest, rsp *ForkResponse) error {
	who, err := author(ctx)
	if err != nil {
		return err
	}
	acc, err := auth.GetAccount(who)
	if err != nil {
		return fmt.Errorf("account not found")
	}
	a, err := ForkApp(req.Slug, req.NewSlug, acc.ID, acc.Name)
	if err != nil {
		return err
	}
	rsp.Result = fmt.Sprintf("Forked to /apps/%s.", a.Slug)
	return nil
}

// ── Embed ───────────────────────────────────────────────────────

type EmbedRequest struct {
	Slug string `json:"slug" required:"true" description:"The app's URL slug"`
}

type EmbedResponse struct {
	Result string `json:"result" description:"An <iframe> tag that puts the app on another page, and a note where the app will not work off this site"`
}

// Embed returns the markup that puts an app on somebody else's page.
//
// This replaced Run, which took a snippet of JavaScript, kept it in memory for
// an hour and handed back an id while promising a URL. Nothing ran — these are
// static pages, the browser runs them — so the verb was wrong about what the
// service does. Create and embed are the two things you do with an app.
// @example {"slug": "pomodoro-timer"}
func (Server) Embed(ctx context.Context, req *EmbedRequest, rsp *EmbedResponse) error {
	who, err := author(ctx)
	if err != nil {
		return err
	}
	a := GetApp(req.Slug)
	// GetApp does not check Public, so a private app is one slug guess away from
	// anybody. Said as "no app called that" rather than "you may not", because
	// the second answer confirms the app exists.
	if a == nil || (!a.Public && a.AuthorID != who) {
		return fmt.Errorf("no app called %q", req.Slug)
	}
	// The raw document does not charge — see handleApp — so a tag pointing at
	// it is a way round the price. Refused rather than quietly given away.
	if a.Price > 0 {
		return fmt.Errorf("%s charges %d credits a use and cannot be embedded: "+
			"the embedded copy would not charge anything", a.Slug, a.Price)
	}
	out := EmbedHTML(origin.Self(), a.Slug, a.Name)
	if bridged(a.RenderHTML()) {
		out += "\n\nThis app calls mu. — the store, a service, the agent. Those " +
			"work on this site, where the page around the frame answers them, and " +
			"nowhere else: elsewhere they wait and then fail."
	}
	rsp.Result = out
	return nil
}

// ── Test ────────────────────────────────────────────────────────

type TestRequest struct {
	Slug string `json:"slug" required:"true" description:"The app's URL slug"`
}

type TestResponse struct {
	Text string `json:"text" description:"Which of the app's API calls work and which fail"`
}

// Test checks an app's HTML and runs its mu.api calls server-side, so an
// author finds out what is broken without opening it.
// @example {"slug": "pomodoro-timer"}
func (Server) Test(ctx context.Context, req *TestRequest, rsp *TestResponse) error {
	who, err := author(ctx)
	if err != nil {
		return err
	}
	result := TestApp(req.Slug, who)
	if result == nil {
		return fmt.Errorf("no app called %q", req.Slug)
	}
	var b strings.Builder
	if result.OK {
		b.WriteString("No problems found.\n")
	}
	for _, issue := range result.Issues {
		b.WriteString("- " + issue + "\n")
	}
	for _, c := range result.APITests {
		switch {
		case c.Error != "":
			b.WriteString("- " + c.Call + ": failed — " + c.Error + "\n")
		case c.Status >= 400:
			b.WriteString(fmt.Sprintf("- %s: HTTP %d\n", c.Call, c.Status))
		default:
			b.WriteString("- " + c.Call + ": ok\n")
		}
	}
	if b.Len() == 0 {
		b.WriteString("The app makes no API calls and has no structural problems.\n")
	}
	rsp.Text = b.String()
	return nil
}
