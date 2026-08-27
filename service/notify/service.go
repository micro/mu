package notify

import (
	"context"
	"fmt"

	"mu/internal/app"
	"mu/internal/service"
)

// Server is the go-micro handler. Its exported methods become the notify_*
// tools.
type Server struct{}

func caller(ctx context.Context) (string, error) {
	id := service.AccountFrom(ctx)
	if id == "" {
		return "", fmt.Errorf("sign in to send or read notifications")
	}
	return id, nil
}

// ── Send ────────────────────────────────────────────────────────

type SendRequest struct {
	// No recipient field, deliberately. See the package comment: a tool that
	// took one would be a way to make a stranger's phone buzz.
	Title string `json:"title" required:"true" description:"The line on the lock screen. Short — this is the whole message on a watch"`
	Body  string `json:"body,omitempty" description:"A second line, up to about 300 characters. Optional"`
	URL   string `json:"url,omitempty" description:"Where tapping it goes, e.g. /inbox. A notification you cannot act on trains someone to ignore the next one"`
}

type SendResponse struct {
	Result string `json:"result" description:"Confirmation, or why it could not be delivered"`
}

// Send notifies the caller on their own devices.
//
// For the thing that has to interrupt: a price crossed, a delivery arrived, a
// standing instruction fired. Not for an answer — an answer belongs in the
// thread the question was asked in, where it can be read at leisure. A
// notification is the only message this product sends that takes somebody's
// attention without being asked for it, and an agent that reaches for it
// routinely is one whose notifications get turned off.
// @example {"title": "ETH below $3,000", "body": "It is at $2,984, down 4% today.", "url": "/markets"}
func (Server) Send(ctx context.Context, req *SendRequest, rsp *SendResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	// "agent" in the record, which is as precise as the context can currently
	// be: it carries the account and the request, not which agent is running.
	// Better a true general answer than a guessed specific one — the page says
	// what interrupted you, and "agent" beside "mail" and "reminder" is already
	// the distinction somebody needs to decide what to turn off.
	if err := Send(owner, req.Title, req.Body, req.URL, "agent"); err != nil {
		return err
	}
	rsp.Result = "Notified."
	return nil
}

// ── Devices ─────────────────────────────────────────────────────

type DevicesRequest struct{}

type Device struct {
	Label string `json:"label,omitempty" description:"What the device said it was, as far as the browser will say"`
	Added string `json:"added" description:"When it was turned on"`
	Last  string `json:"last,omitempty" description:"What became of the last notification sent here"`
}

type DevicesResponse struct {
	Devices   []Device `json:"devices" description:"Every device this account can be reached on"`
	Reachable bool     `json:"reachable" description:"Whether a notification sent right now would have anywhere to go"`
}

// Devices says whether this account can be reached, and on what.
//
// So an agent can find out before it promises to tell somebody something. A
// standing instruction that ends in a notification nobody can receive is worse
// than one that was refused up front, because it looks like it is working.
// @example {}
func (Server) Devices(ctx context.Context, req *DevicesRequest, rsp *DevicesResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	for _, d := range Devices(owner) {
		one := Device{Label: d.Label, Added: app.TimeAgo(d.Added)}
		switch {
		case d.Failed.After(d.Sent) && d.Error != "":
			one.Last = "failed " + app.TimeAgo(d.Failed) + ": " + d.Error
		case !d.Sent.IsZero():
			one.Last = "delivered " + app.TimeAgo(d.Sent)
		}
		rsp.Devices = append(rsp.Devices, one)
	}
	rsp.Reachable = Reachable(owner)
	return nil
}

// LoadService registers the service.
func LoadService() {
	if err := service.Register(Spec); err != nil {
		app.Log("notify", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:        "notify",
	Handler:     new(Server),
	Description: "Reach yourself when you are not looking at the page",
	Page:        "/notify",
	Icon:        "notify.svg",
	Scoped:      true,
	Endpoints: map[string]service.Endpoint{
		// Account, not merely priced, and for the same reason sms_send is:
		// what this spends is somebody's attention, and an anonymous caller
		// paying for the privilege of interrupting a stranger is still
		// interrupting a stranger. There is no recipient argument either, so
		// even an account-scoped caller can only reach itself.
		//
		// Writes rather than Destructive: nothing is lost and nothing is
		// charged. It does take attention, which is why the doc says when not
		// to use it.
		"Send": {Writes: true, Needs: service.Account,
			Doc: "Interrupt yourself on your own devices — a phone, a laptop — with the page closed. " +
				"For the thing that cannot wait until the next time somebody looks. An answer to a " +
				"question belongs in the thread it was asked in, not here"},
		"Devices": {Needs: service.Account,
			Doc: "Whether this account can be reached by notification at all, and on which devices. " +
				"Worth asking before promising to tell somebody something"},
	},
}
