package user

// What an account does about other people's content.
//
// Save, hide, flag, block: six things a caller does that are about somebody
// else's post and nobody else's business. They were tools with no service
// behind them — registered in internal/api pointing at /app/save, /app/block
// and the rest, so calling one built an HTTP request and pushed it through the
// mux to reach a page handler. They had nowhere else to go, because the package
// that registered them is not allowed to import anything that could do the
// work.
//
// They are a service because they are one: a noun (the caller), six verbs, one
// caller's data, and an answer that does not depend on which door the request
// came through. The store is internal/app's prefs file and internal/flag's
// counter, both of which already existed and are unchanged — what was missing
// was a front door that is not a web page.
//
// Naming: this is the caller acting on their own account, which is why Block
// and Save sit together. Flag is the one that leaves: it puts an item in front
// of a human moderator rather than changing anything for the caller. It is here
// anyway, because from the caller's side it is the same gesture as hiding
// something — the difference is who else sees it, and that is the moderator's
// business, not a second service.

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/app"
	"mu/internal/flag"
	"mu/internal/service"
)

// Server is the go-micro handler. Every method is account-scoped: there is no
// such thing as blocking somebody on behalf of nobody.
type Server struct{}

func caller(ctx context.Context) (string, error) {
	id := service.AccountFrom(ctx)
	if id == "" {
		return "", fmt.Errorf("sign in to do that")
	}
	return id, nil
}

// item is the shape shared by save, unsave, hide and flag: a kind and an id.
type item struct {
	Type string `json:"type" required:"true" description:"What kind of thing it is: post, work or app"`
	ID   string `json:"id" required:"true" description:"The item's id, as shown wherever you found it"`
}

func (i item) check() error {
	if i.Type == "" || i.ID == "" {
		return fmt.Errorf("type and id are both required")
	}
	return nil
}

// ── Save ────────────────────────────────────────────────────────

type SaveRequest struct{ item }

type SaveResponse struct {
	Result string `json:"result" description:"Confirmation"`
}

// Save bookmarks an item so the caller can find it again.
// @example {"type": "post", "id": "abc123"}
func (Server) Save(ctx context.Context, req *SaveRequest, rsp *SaveResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	if err := req.check(); err != nil {
		return err
	}
	app.SaveItem(who, req.Type, req.ID)
	rsp.Result = "Saved."
	return nil
}

// ── Unsave ──────────────────────────────────────────────────────

type UnsaveRequest struct{ item }

type UnsaveResponse struct {
	Result string `json:"result" description:"Confirmation"`
}

// Unsave forgets that an item was bookmarked. The item itself is untouched.
// @example {"type": "post", "id": "abc123"}
func (Server) Unsave(ctx context.Context, req *UnsaveRequest, rsp *UnsaveResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	if err := req.check(); err != nil {
		return err
	}
	app.UnsaveItem(who, req.Type, req.ID)
	rsp.Result = "Removed from saved."
	return nil
}

// ── Hide ────────────────────────────────────────────────────────

type HideRequest struct{ item }

type HideResponse struct {
	Result string `json:"result" description:"Confirmation"`
}

// Hide stops the caller seeing an item. It affects this account's view and
// nobody else's — to report something, use Flag.
// @example {"type": "post", "id": "abc123"}
func (Server) Hide(ctx context.Context, req *HideRequest, rsp *HideResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	if err := req.check(); err != nil {
		return err
	}
	app.DismissItem(who, req.Type, req.ID)
	rsp.Result = "Hidden from your view."
	return nil
}

// ── Flag ────────────────────────────────────────────────────────

type FlagRequest struct{ item }

type FlagResponse struct {
	Result string `json:"result" description:"Confirmation, and whether this item was already reported"`
	Count  int    `json:"count" description:"How many people have reported it"`
}

// Flag reports an item for a human moderator. It removes nothing by itself.
// @example {"type": "post", "id": "abc123"}
func (Server) Flag(ctx context.Context, req *FlagRequest, rsp *FlagResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	if err := req.check(); err != nil {
		return err
	}
	count, already, err := flag.Add(req.Type, req.ID, who)
	if err != nil {
		return err
	}
	rsp.Count = count
	if already {
		rsp.Result = "You had already reported this."
		return nil
	}
	// Enough reports hide the item, and the cache the pages read has to be told.
	if count >= 3 {
		if deleter, ok := flag.GetDeleter(req.Type); ok {
			deleter.RefreshCache()
		}
	}
	rsp.Result = "Reported for moderation."
	return nil
}

// ── Block ───────────────────────────────────────────────────────

type BlockRequest struct {
	User string `json:"user" required:"true" description:"The username to block"`
}

type BlockResponse struct {
	Result string `json:"result" description:"Confirmation"`
}

// Block hides everything from another account.
// @example {"user": "someone"}
func (Server) Block(ctx context.Context, req *BlockRequest, rsp *BlockResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	if req.User == "" {
		return fmt.Errorf("user is required")
	}
	if req.User == who {
		return fmt.Errorf("you cannot block yourself")
	}
	app.BlockUser(who, req.User)
	rsp.Result = "Blocked @" + req.User + "."
	return nil
}

// ── Unblock ─────────────────────────────────────────────────────

type UnblockRequest struct {
	User string `json:"user" required:"true" description:"The username to stop blocking"`
}

type UnblockResponse struct {
	Result string `json:"result" description:"Confirmation"`
}

// Unblock reverses Block, so their posts reach the caller again.
// @example {"user": "someone"}
func (Server) Unblock(ctx context.Context, req *UnblockRequest, rsp *UnblockResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	if req.User == "" {
		return fmt.Errorf("user is required")
	}
	app.UnblockUser(who, req.User)
	rsp.Result = "Unblocked @" + req.User + "."
	return nil
}

// Load registers the service.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("user", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:    "user",
	Handler: new(Server),
	// "User Prefs" rather than "User": what this holds is the set of decisions
	// one account has made about everybody else's content, and "User" on its own
	// reads like a directory of people, which it is not.
	Label:       "User Prefs",
	Description: "What you keep, hide and block — your own view of everybody else's content",
	Page:        "/user",
	Icon:        "account.png",
	Scoped:      true,
	Endpoints: map[string]service.Endpoint{
		"Saved":   {Aliases: []string{"saved_list"}, Doc: "List the items the caller has saved for later, with their links"},
		"Save":    {Aliases: []string{"content_save", "save"}, Doc: "Save an item to the caller's bookmarks so it can be found again. Private to the caller, and reversible with user_unsave"},
		"Unsave":  {Aliases: []string{"content_unsave", "unsave"}, Doc: "Remove an item from the caller's bookmarks. Leaves the item itself untouched — this only forgets that it was saved", Destructive: true},
		"Hide":    {Aliases: []string{"content_hide", "dismiss"}, Doc: "Hide an item so the caller stops seeing it. Affects only this account's view; use user_flag to report something to a moderator instead"},
		"Flag":    {Aliases: []string{"content_flag", "flag"}, Doc: "Report a post, comment or message for a human moderator to look at. Does not remove anything itself — use user_hide to hide something from your own view"},
		"Block":   {Aliases: []string{"block_user"}, Doc: "Block another account, hiding all of their content from the caller's view", Destructive: true},
		"Unblock": {Aliases: []string{"unblock_user"}, Doc: "Stop blocking another account, so their posts and messages reach the caller again. Reverses user_block"},
	},
}

// Delete removes everything this account chose to save, hide or block.
//
// Called by the account-deletion hook. The store it clears is internal/app's,
// and that hook already existed pointing straight at it — which was fine until
// there was a service that owned the data, at which point a deletion routed
// around its owner is a second place to remember.
func Delete(owner string) { app.ClearUserPrefs(owner) }

// ── Saved ───────────────────────────────────────────────────────

// Listing what you saved was a tool with no service behind it, while saving
// and unsaving were pointed at pages. All three are here now, which is the
// point: a caller that can save something can ask what it saved.

type SavedRequest struct {
	Limit int `json:"limit" description:"Max results (default 50)"`
}

type SavedResponse struct {
	Text string `json:"text" description:"Saved items: what each is, and a link"`
}

// Saved lists the items the caller bookmarked, newest first.
// @example {}
func (Server) Saved(ctx context.Context, req *SavedRequest, rsp *SavedResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	limit := req.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	entries := app.GetSavedList(who)
	if len(entries) == 0 {
		rsp.Text = "You have not saved anything yet."
		return nil
	}
	var b strings.Builder
	for i, e := range entries {
		if i >= limit {
			break
		}
		title := e.Title
		if title == "" {
			title = e.Type + " " + e.ID
		}
		b.WriteString("- " + title + "\n  " + e.URL + "\n")
	}
	rsp.Text = b.String()
	return nil
}
