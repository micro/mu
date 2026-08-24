package server

// What each building block asks of the others, wired in one place.
//
// Packages here do not import each other: blog does not know about admin, mail
// does not know about the agent, wallet does not know about auth's opinion of
// it. They expose a function variable and this fills it in, which is why the
// dependency graph stays a tree and any one of them can be read alone.
//
// It lives here rather than in main because main is a command — flags, and
// which of the two programs (server or CLI) to run.

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"mu/account"
	"mu/admin"
	"mu/agent"
	agentblog "mu/agent/blog"
	"mu/agent/digest"
	mailagent "mu/agent/mail"
	"mu/agent/micro"
	"mu/agent/moderate"
	agentsocial "mu/agent/social"
	"mu/agent/work"
	help "mu/docs"
	"mu/home"
	"mu/inbox"
	"mu/internal/ai"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/backup"
	"mu/internal/data"
	"mu/internal/event"
	"mu/internal/google"
	"mu/internal/notes"
	"mu/internal/origin"
	"mu/internal/push"
	"mu/internal/quota"
	"mu/internal/service"
	"mu/internal/settings"
	"mu/internal/thread"
	"mu/internal/user"
	"mu/internal/x402"
	"mu/service/apps"
	"mu/service/blog"
	"mu/service/contacts"
	"mu/service/docs"
	"mu/service/events"
	"mu/service/files"
	"mu/service/images"
	"mu/service/mail"
	"mu/service/markets"
	"mu/service/news"
	"mu/service/recall"
	"mu/service/shell"
	"mu/service/sms"
	"mu/service/social"
	"mu/service/stream"
	"mu/service/tasks"
	"mu/service/wallet"
	"mu/service/web"
)

// mailHistoryTurns is how much of an email thread an agent is reminded of.
//
// Six messages is three exchanges, which covers the back-and-forth people
// actually have by mail and stops a thread somebody has been adding to for a
// month from costing more in prompt than the answer is worth.
const mailHistoryTurns = 6

// wireHooks connects the building blocks to each other.
func wireHooks() {
	// Attach the calendar somebody already keeps. events owns scheduling and
	// knows nothing about Google; this is the only place the two meet, and it
	// stays unset on an instance with no Google credentials — one calendar
	// instead of two, rather than a broken second one.
	google.Load()
	if google.Configured() {
		events.ExternalConnected = func(owner string) bool {
			return google.HasScope(owner, google.CalendarScope)
		}
		events.ExternalAccount = google.ConnectedEmail
		events.ExternalBusy = func(owner string, from, to time.Time) []events.Slot {
			periods, err := google.Busy(owner, from, to)
			if err != nil {
				// A calendar that cannot be read must not make "when am I
				// free" fail. The answer narrows to what Mu knows, which is
				// what it was before this was wired at all.
				if err != google.ErrNotConnected {
					app.Log("events", "google busy for %s: %v", owner, err)
				}
				return nil
			}
			slots := make([]events.Slot, 0, len(periods))
			for _, p := range periods {
				slots = append(slots, events.Slot{Start: p.Start, End: p.End})
			}
			return slots
		}
		contacts.ExternalConnected = func(owner string) bool {
			return google.HasScope(owner, google.ContactsScope)
		}
		contacts.ExternalFind = func(owner, query string) []contacts.External {
			people, err := google.SearchContacts(owner, query, 10)
			if err != nil {
				if err != google.ErrNotConnected {
					app.Log("contacts", "google contacts for %s: %v", owner, err)
				}
				return nil
			}
			out := make([]contacts.External, 0, len(people))
			for _, p := range people {
				out = append(out, contacts.External{Name: p.Name, Email: p.Email, Phone: p.Phone})
			}
			return out
		}
		events.ExternalEntries = func(owner string, from, to time.Time) []events.External {
			entries, err := google.Events(owner, from, to, 25)
			if err != nil {
				if err != google.ErrNotConnected {
					app.Log("events", "google events for %s: %v", owner, err)
				}
				return nil
			}
			out := make([]events.External, 0, len(entries))
			for _, e := range entries {
				out = append(out, events.External{
					Title: e.Title, Start: e.Start, End: e.End,
					Location: e.Location, AllDay: e.AllDay, Source: events.ExternalName,
				})
			}
			return out
		}
	}

	events.OnFire = func(accountID, title, note string) {
		// Nothing goes to the public timeline.
		//
		// This posted the reminder's title and the owner's account id once, so
		// "Dentist about the biopsy results" was published to the open internet
		// the moment it fired. That was cut back to a contentless line, which
		// was the wrong repair: /stream is served with no session, so even a
		// bare "a reminder fired" tells anybody watching that somebody here was
		// reminded of something, at that minute.
		push.Send(accountID, push.Notification{Title: "⏰ " + title, Body: note, URL: "/events"})
	}
	// Work an agent is asked to do — a task assigned, a standing instruction
	// falling due — reaches it on the bus rather than through this file.
	//
	// tasks.RunAgent, events.RunAgent and events.OnFireEvent were three
	// function variables filled in here, which is a service running an agent
	// with the import hidden from the compiler. They are gone; agent/work
	// subscribes, and both services announce and know nothing about who
	// listens.
	//
	// The thing that made them worth having is unchanged and is what a server
	// that stays up can do where a stdio MCP process cannot: "every morning,
	// brief me and mail it" has nowhere to live in a process that only exists
	// while a client is attached.
	work.Load()

	// Mail is a client like another client: it speaks its own protocol and
	// hands what arrives to the agent. See agent/mail.
	mailagent.Load()

	// And the agent introduces itself to a new account, in that account's
	// inbox. Onboarding as a message rather than a page: the claim is that you
	// hand work to an agent and it answers where you asked, and the way to make
	// that claim is to do it before anything else does. See agent/mail/welcome.go.
	mailagent.Welcome()

	// And whether what people publish should stay up. A judgement, so it is an
	// agent — it was a function variable inside internal/flag that service/chat
	// filled in, which put content moderation for the whole instance behind an
	// unrelated service loading. See agent/moderate.
	moderate.Load()

	// Telling the operator when something is worth knowing. After the mail
	// agent, because it delivers to an inbox here. See admin/alert.go.
	admin.Watch()

	// When an event is scheduled, email the owner an .ics invite so it also
	// lands in their real calendar. Only for users with a verified email (e.g.
	// via Google sign-in) and only when this instance can send mail.
	events.OnCreate = func(e *events.Event) {
		domain := mail.ConfiguredDomain()
		if domain == "" || domain == "localhost" {
			return
		}
		acc, err := auth.GetAccount(e.Owner)
		if err != nil || acc.Email == "" || !acc.EmailVerified {
			return
		}
		when := e.When.Local().Format("Mon 2 Jan 2006, 15:04 MST")
		body := fmt.Sprintf(`<p>You asked Mu to remind you:</p><p class="status-icon"><strong>%s</strong><br>%s</p>`,
			html.EscapeString(e.Title), html.EscapeString(when))
		if e.Note != "" {
			body += `<p>` + html.EscapeString(e.Note) + `</p>`
		}
		body += `<p class="text-muted text-sm">Added to your calendar from the attached invite.</p>`
		ics := events.ICS(e, acc.Email)
		if _, err := mail.SendCalendarInvite("Mu Events", "no-reply@"+domain, acc.Email, "Event: "+e.Title, body, ics); err != nil {
			app.Log("events", "calendar invite to %s failed: %v", acc.Email, err)
		}
	}
	app.ToolCountFunc = api.ToolCount

	// Mail arriving is a fact on the bus, not a call into the agent.
	//
	// service/mail used to declare OnNewMail and this filled it in — a service
	// reaching up into the product, which is the direction the layering
	// forbids. It publishes event.MailReceived now and knows nothing
	// about who listens. This is the listener.
	go func() {
		sub := event.Subscribe(event.MailReceived)
		for e := range sub.Chan {
			// mail.MessageFrom, not four type assertions on a bag of strings.
			//
			// The bag is what this used to read, and it was one of two shapes
			// being published on this topic — the other being the whole message
			// as JSON, which the recorder that writes mail into internal/thread
			// reads. Every subscriber understood one of the two, so which mail
			// reached the record depended on which door it came in by. There is
			// one shape now and this is how it is decoded.
			m, ok := mail.MessageFrom(e.Data)
			if !ok {
				continue
			}
			accountID, from, subject := m.Owner, m.From, m.Subject
			if accountID == "" {
				continue
			}
			// Nothing goes to the public timeline.
			//
			// /stream is served with no session, so even a bare "mail arrived"
			// tells anybody watching that somebody here was written to, at that
			// minute — and correlated with a message you sent yourself, that it
			// was delivered.
			//
			// The device, which needs nothing linked and works with the page
			// closed. See internal/push. There were three other notifiers here
			// and feeding them cost a model call on every arriving message, to
			// write a summary line they shared. Push does the job without one.
			title := subject
			if strings.TrimSpace(title) == "" {
				title = "New mail"
			}
			push.Send(accountID, push.Notification{
				Title: title,
				Body:  "From " + from,
				URL:   "/inbox",
				Tag:   "mail-" + from,
			})
		}
	}()

	// Where a push service should complain.
	//
	// This instance's own address. It was mailto:support@<domain>, and there is
	// no support@ any more — it was the one address the inbound whitelist did
	// not apply to, so it was the one address spam could reach. A URL is a valid
	// VAPID contact, and it is the honest one: a push service with a problem
	// should look at the instance, not write to a mailbox nobody reads.
	push.Contact(origin.Self)

	// Resolve app author display names server-side from the authenticated
	// account, so the native apps.Build service never trusts a model-supplied
	// author name.
	apps.AuthorNameFor = func(accountID string) string {
		if acc, err := auth.GetAccount(accountID); err == nil && acc.Name != "" {
			return acc.Name
		}
		return accountID
	}

	// Wire social context into news article views
	news.FetchSocialContext = func(articleURL, articleContent string) string {
		ctx := social.FetchContext(articleURL, articleContent)
		return social.RenderContextHTML(ctx)
	}

	// The record follows a renamed account.
	//
	// Claiming an unclaimed account changes its id, and the id is what every
	// conversation is filed under. internal/auth sits below internal/thread and
	// cannot call it, so the store registers instead — see auth.Renamed. Without
	// this, somebody invited to keep the conversation they had by email would
	// sign up and find nothing, which is the one thing the invitation promises.
	auth.Renamed(thread.Rename)

	// load the home cards
	// What the inbox needs from packages it must not import. It renders the
	// record; the roster is the agent's and the mail domain is the mail
	// service's, and neither is a reason for a page over internal/thread to
	// depend on them. See inbox/doc.go.
	// One line from the agent onto a conversation — the acknowledgement when
	// work is handed over. The agent owns how anything it says is written
	// down, so the inbox asks rather than writing to the record itself.
	inbox.AgentSaid(func(accountID, threadID, text string) {
		agent.Answered(accountID, threadID, text, "")
	})
	inbox.AgentName = agent.NameOf
	inbox.Address = mail.SharedAgentAddress

	// Somebody's own address, for the Write button on their profile.
	//
	// internal/user is below the mail service and may not import it — see the
	// layering rule — and inbox.Address above is the same hook for the same
	// reason. Their address without a tag, which is the person: service/mail
	// will not wake an agent on untagged mail, so writing to somebody here is
	// writing to them.
	user.AddressFor = func(accountID string) string {
		return mail.EmailForUser(accountID, mail.ConfiguredDomain())
	}
	// The roster, so the inbox can offer a box per agent rather than only the
	// ones that already have mail — and so a box is the agent's address tag
	// rather than a second slug derived from its name. See inbox.Agents.
	// What to call the agent an account gets without making one, so the assign
	// picker reads as a list of agents rather than "default" and some names.
	if a := agent.Platform(""); a != nil {
		inbox.DefaultAgentName = a.Name
	}

	inbox.Agents = func(owner string) []inbox.Agent {
		var out []inbox.Agent
		for _, a := range agent.Agents(owner) {
			out = append(out, inbox.Agent{ID: a.ID, Name: a.Name, Tag: a.Tag})
		}
		return out
	}

	// The specialist for a service, on that service's page. Names match by
	// design — the weather agent is the one that calls the weather tools — so
	// this is a lookup rather than a table, and a service with no specialist
	// gets no line rather than a link to the general one.
	api.AgentPrompts = func(name string) (string, []string) {
		a := micro.Get(name)
		if a == nil {
			return "", nil
		}
		return agent.Path("", a.ID), a.Examples
	}

	home.Load()

	// load agent
	agent.Load()

	// Wire user context into the agent — personalises responses.
	// What an agent knows about you before you have said anything.
	//
	// It was four lines — unread mail, balance, top movers, notes — and none of
	// them answered the question every specialist actually needs answered
	// first, which is who and where and when. So the weather agent, asked "do I
	// need a coat today", replied "which city are you in?" from an instance
	// whose home screen was showing that account's local forecast: the
	// coordinates existed, in a browser, and nothing server-side had ever been
	// told. Places could not do "near me", prayer had no latitude to compute
	// from, and every scheduled run started from nowhere because there was no
	// browser in the room at 7am.
	//
	// Who, where, when, then what is going on. The order is deliberate: the
	// first three are the ones a model needs to not ask a question back.
	userCtxFunc := func(accountID string) string {
		var parts []string
		if acc, err := auth.GetAccount(accountID); err == nil && acc != nil {
			if name := strings.TrimSpace(acc.Name); name != "" {
				parts = append(parts, "- You are talking to "+name+" (@"+acc.ID+")")
			}
		}
		// Where they are, and what time it is there. Together, because either
		// alone still leaves "today" ambiguous — see account/place.go.
		if place := account.PlaceLine(accountID); place != "" {
			parts = append(parts, "- They are in "+place)
		}
		parts = append(parts, "- It is "+localNow(accountID))
		// Unread mail count.
		if unread := mail.GetUnreadCount(accountID); unread > 0 {
			parts = append(parts, fmt.Sprintf("- %d unread email(s)", unread))
		}
		// Wallet balance.
		bal := account.Balance(accountID)
		if bal > 0 {
			parts = append(parts, fmt.Sprintf("- Wallet: %d credits", bal))
		}
		// Market prices — top movers.
		if prices := markets.TopMovers(3); prices != "" {
			parts = append(parts, "- Markets: "+prices)
		}
		// Persistent memory — things the user has told you to remember.
		if mem := notes.ForContext(accountID); mem != "" {
			parts = append(parts, "User preferences/notes:\n"+mem)
		}
		if len(parts) == 0 {
			return ""
		}
		return strings.Join(parts, "\n")
	}
	agent.UserContextFunc = userCtxFunc
	// The home cards, as something an agent can read — sent with every question,
	// because they are what the reader is looking at.
	agent.CardContextFunc = func(accountID string) string {
		acc, err := auth.GetAccount(accountID)
		if err != nil || acc == nil {
			return ""
		}
		return home.CardContext(acc)
	}

	// Three hooks stood here handing the digest a way to publish. They were the
	// cost of a service that could not import the blog; the digest is an agent
	// now and imports it.
	digest.Load()

	// load search
	web.Load()

	// load docs
	help.Load()

	// Load the stream (platform event timeline).
	stream.Load()

	// Keep copies of the data directory. It takes one at startup, because the
	// most useful snapshot is the one from before whatever is about to go
	// wrong, and the search index goes in through VACUUM rather than a file
	// copy — nothing reindexes what is already there, so losing it is not a
	// rebuild, it is a loss.
	backup.IndexSnapshot = data.SnapshotInto
	backup.Load()

	// Optionally run go-micro's MCP gateway alongside mu's existing /mcp, on a
	// separate port. It auto-exposes every registered service as an MCP tool.
	// Off unless MCP_GATEWAY_ADDR is set — additive, no change to /mcp.
	if addr := settings.Get("MCP_GATEWAY_ADDR"); addr != "" {
		go func() {
			app.Log("main", "starting go-micro MCP gateway on %s", addr)
			if err := service.StartMCPGateway(addr); err != nil {
				app.Log("main", "MCP gateway stopped: %v", err)
			}
		}()
	}

	// Wire user → blog callback (avoids direct import between building blocks)
	user.GetUserPosts = func(authorID, authorName string) []user.UserPost {
		posts := blog.PostsByAuthorID(authorID, authorName)
		result := make([]user.UserPost, len(posts))
		for i, p := range posts {
			result[i] = user.UserPost{
				ID:        p.ID,
				Title:     p.Title,
				Content:   p.Content,
				CreatedAt: p.CreatedAt,
				Private:   p.Private,
			}
		}
		return result
	}
	user.LinkifyContent = blog.Linkify

	user.GetUserApps = func(authorID string) []user.UserApp {
		appList := apps.ByAuthor(authorID)
		result := make([]user.UserApp, len(appList))
		for i, a := range appList {
			result[i] = user.UserApp{
				Slug:        a.Slug,
				Name:        a.Name,
				Description: a.Description,
				Icon:        a.Icon,
			}
		}
		return result
	}

	// Wire admin → blog callbacks (avoids blog importing admin)
	admin.GetNewAccountBlog = blog.PostsByNewAccounts
	admin.RefreshBlogCache = blog.RefreshCache

	// Register account deletion hooks — each package cleans up its own data.
	auth.AccountDeleteHooks = append(auth.AccountDeleteHooks,
		blog.DeletePostsByAuthor,
		social.DeleteByAuthor,
		apps.DeleteAppsByAuthor,
		stream.DeleteByAccount,
		mail.DeleteInbox,
		func(id string) { account.DeleteCredits(id) },
		func(id string) { wallet.DeleteBaseWallet(id) },
		func(id string) { micro.DeleteUserAgents(id) },
		// The devices they told us to notify. A subscription outliving the
		// account is a stranger's phone still receiving somebody's mail.
		push.Forget,
		notes.Clear,

		// Everything the caller stored themselves. These six were missing, so
		// deleting an account left behind its files, address book, calendar,
		// tasks, records and generated images — all of them account-scoped, all
		// of them still on disk under an owner who no longer existed. Nothing
		// noticed because nothing could ask: userdb had no delete-by-owner, so
		// there was no function for a hook to call.
		files.DeleteAll,
		contacts.DeleteAll,
		tasks.DeleteAll,
		events.DeleteAll,
		images.DeleteAll,
		docs.DeleteAll,
		sms.DeleteAll,
		// And their machine, which is not a file on this disk but a container
		// and a volume. Nothing else in this list reaches outside the data
		// directory; this one has to, because that is where the caller's work
		// actually is.
		shell.DeleteMachine,

		// Everything that was ever said, on any client. The record is written
		// by the machinery rather than by a service, so nothing owned it and
		// nothing cleared it: deleting an account left the transcript of every
		// conversation it had ever had on disk. recall is the reader over it,
		// and the only thing in the catalogue that knows it exists.
		recall.Delete,
	)

	// Enable indexing after all content is loaded
	// This allows the priority queue to process new items first
	data.StartIndexing()

	// Start web search topics (loads cache from disk, generates in background)
	web.StartTopics()

	// The instance's own agent needs an account before it does anything, so
	// what it does can be attributed. No-op until a human admin exists — the
	// first signup on a fresh instance is theirs, not the agent's.
	auth.EnsureMicro()

	// Start daily opinion generation (publishes as blog post)
	agentblog.Start()

	// Start surfacing breaking stories into social
	agentsocial.Start()

	// Start the notes loop — Mu's own story, posted to its own blog as the
	// system account (low cadence; disable with NOTES=off).
	blog.StartNotes()

	// Wire guest agent news search directly to the live feed-backed provider path.
	api.GuestNewsSearch = news.SearchToolText

	// What a plan is worth, to the two places that enforce it.
	//
	// Both live below wallet/ — internal/auth and agent/ — and neither may
	// import it, so the numbers come down through here like every other cycle
	// in this file. Until this existed the pricing page sold "5 agents" and
	// "higher rate limits" to a product where no code had ever read a plan.
	//
	// An admin is exempt from the agent cap for the same reason they are exempt
	// from the charge: they are the operator, and the operator is already
	// paying for the instance.
	// Everything a subscription used to sell is keyed on accountability now,
	// which is auth.Trusted: an admin, an approved account, a verified address,
	// or money on the balance. Plans are gone — they sold three limits and a
	// credit at par, which is nothing a top-up does not do — and the limits they
	// sold were never really tiers. They are abuse controls, and what makes
	// abuse expensive is a real person behind the account, not a subscription.
	//
	// So the axis is the one that was already there: put money in, or prove who
	// you are, and the caps lift. It costs nothing to build, needs no Stripe
	// object, and it is the same signal that decides who may post publicly and
	// who may send mail out.
	agent.AgentAllowance = func(accountID string) int {
		acc, err := auth.GetAccount(accountID)
		if err != nil {
			return 0
		}
		if acc.Admin || acc.Agent || auth.Trusted(accountID) {
			return 0 // no limit
		}
		return 1
	}

	// The gateway every service call goes through.
	//
	// internal/service cannot ask these questions itself — the answers need the
	// wallet, and the wallet sits above it — so the two halves are filled in
	// here, the same way every other cycle in this file is broken. See
	// internal/service/gateway.go for what it replaced: four different places a
	// charge could live, and most operations landing in none of them.
	service.Gate.Allow = func(account, op string) (bool, error) {
		// Somebody who paid in USDC has already paid, at the door, for this
		// exact operation — VerifyAndSettle runs before the tool does, and the
		// free trial is counted there too. There is no account behind a wallet
		// identity by design: not signing up is the entire point of x402.
		//
		// So asking the wallet about one gets "account not found", and the
		// gateway turns that into a refusal — of a call that has been paid for.
		// The money is gone and the caller has nothing. This was live for every
		// scoped priced service, and binding the caller on priced tools spread
		// it to web_search, which is the example in the README.
		//
		// Allowed, recorded, and not charged again.
		if api.IsWalletIdentity(account) {
			quota.Record(account, op)
			return false, nil
		}

		// A daily limit is checked before anything about money, because it is
		// not about money. It is the second control: a price stops somebody who
		// has to pay and does nothing about a loop, and what a loop spends on
		// the three outbound operations is a domain's or a number's reputation,
		// which no balance repairs. See the limit block in quota.json.
		if over, why := quota.OverLimit(account, op); over {
			return false, fmt.Errorf("%s", why)
		}

		ok, _, cost, err := quota.CheckQuota(account, op)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, fmt.Errorf("this costs %d credits and your balance is %d — top up at /billing/topup",
				cost, quota.BalanceOf(account))
		}
		return true, nil
	}
	service.Gate.Charge = func(account, op string) {
		if err := quota.Charge(account, op, nil); err != nil {
			app.Log("wallet", "charging %s for %s: %v", account, op, err)
		}
	}
	// Served without reaching a paid provider: recorded, because /usage should
	// show what an account did, and not charged, because it cost nothing to
	// answer. See internal/service/meter.go.
	service.Gate.Free = func(account, op string) { quota.Record(account, op) }
	// One counter, moved once, after the call succeeded — it is what both the
	// free allowance and the daily limit read.
	service.Gate.Done = quota.Done

	// How hard an account's agents may hit this at once.
	service.Concurrency = func(account string) int {
		acc, err := auth.GetAccount(account)
		if err != nil {
			return 0
		}
		if acc.Admin || acc.Agent {
			return 0 // the operator's own instance is not queued behind anything
		}
		if auth.Trusted(account) {
			return 16
		}
		// A signup that has put nothing in gets enough to work with and not
		// enough to fan out across the whole catalogue at once.
		return 4
	}

	// Email out goes through this instance's own SMTP unless an operator has
	// configured a provider. No second credential is needed to send from a
	// subdomain of a domain this instance already signs for — DMARC alignment
	// is relaxed by default, so a signature for MAIL_DOMAIN covers a From on a
	// subdomain of it.

	// Who is not capped at all. quota.json holds the number everybody else
	// takes.
	quota.LimitOverride = func(account, op string) (int, bool) {
		acc, err := auth.GetAccount(account)
		if err != nil {
			return 0, false
		}
		// The operator's own instance and its agent are not capped, for the same
		// reason they are not charged. Everybody else takes quota.json's number,
		// which is an abuse ceiling rather than a tier — an operator who wants a
		// different one edits the file that already holds the prices.
		if acc.Admin || acc.Agent {
			return quota.NoLimit, true
		}
		return 0, false
	}

	// Wire MCP quota checking using wallet credit system
	api.QuotaCheck = func(r *http.Request, op string) (bool, int, error) {
		// Nothing to charge, nobody to charge it to. A free tool has no
		// business asking who is calling: news, web fetch, quran and video
		// search are priced at zero because nothing bills us for them, and an
		// anonymous caller was being turned away from all four with "this call
		// is metered". It is not.
		//
		// This is the same mistake as the x402 gate in the HTTP layer, made
		// independently here, which is why fixing that one alone left free
		// tools unreachable. Both now ask what it costs before asking who you
		// are.
		if !quota.Metered(op) {
			// Open, not unguarded. Credits price what a call costs us and rate
			// limits stop bots — see the cost block in internal/quota. A free call
			// is charged nothing, so the limit is the only one of the two
			// doing any work here, and it applies to guests because a
			// signed-in caller is already accountable.
			if _, err := auth.GetSession(r); err != nil && !app.GuestCallAllowed(app.ClientIP(r)) {
				return false, 0, fmt.Errorf("too many free calls from this address — " +
					"sign in at /token to keep going, or wait a few minutes")
			}
			return true, 0, nil
		}
		// Check for x402 payment (bypasses auth + credits).
		// Free trial: first 10 calls per wallet address are free —
		// no payment header needed if within the trial.
		if r.Context().Value(x402.X402ContextKey) != nil {
			// Try free trial first (by wallet address from payment header).
			payAddr := r.Header.Get("X-Wallet-Address")
			if payAddr != "" && x402.UseTrialCall(payAddr) {
				return true, 0, nil
			}
			// Verify, do not settle. The money moves once there is an answer
			// to hand back — see x402.Finish. Everything that can refuse a
			// caller happens here, so a verified payment is a promise that
			// settling will work rather than a charge already taken.
			if _, err := x402.Verify(r, op, r.URL.Path); err != nil {
				return false, 0, fmt.Errorf("x402 payment failed: %w", err)
			}
			return true, 0, nil
		}
		sess, err := auth.GetSession(r)
		if err != nil {
			// Not "authentication required", which is what an account-scoped
			// tool answers with a 401 and a WWW-Authenticate header telling a
			// client where to sign in. This is a different condition wearing
			// the same words: the call is metered, and there is nobody to
			// charge. Signing in is one answer; paying is the other, and a
			// client told to authenticate would never find the second.
			return false, 0, fmt.Errorf("this call is metered: sign in so it can be charged to your credits, or send an x402 payment")
		}
		canProceed, _, cost, err := quota.CheckQuota(sess.Account, op)
		return canProceed, cost, err
	}

	// Wire agent quota checking (same wallet credit system)
	agent.QuotaCheck = func(r *http.Request, op string) (bool, int, error) {
		// Free is free here too — the third copy of this decision. See the
		// note on api.QuotaCheck above.
		if !quota.Metered(op) {
			return true, 0, nil
		}
		// Check for x402 payment (bypasses auth + credits)
		if r.Context().Value(x402.X402ContextKey) != nil {
			// Verify, do not settle. The money moves once there is an answer
			// to hand back — see x402.Finish. Everything that can refuse a
			// caller happens here, so a verified payment is a promise that
			// settling will work rather than a charge already taken.
			if _, err := x402.Verify(r, op, r.URL.Path); err != nil {
				return false, 0, fmt.Errorf("x402 payment failed: %w", err)
			}
			return true, 0, nil
		}
		sess, err := auth.GetSession(r)
		if err != nil {
			// Not "authentication required", which is what an account-scoped
			// tool answers with a 401 and a WWW-Authenticate header telling a
			// client where to sign in. This is a different condition wearing
			// the same words: the call is metered, and there is nobody to
			// charge. Signing in is one answer; paying is the other, and a
			// client told to authenticate would never find the second.
			return false, 0, fmt.Errorf("this call is metered: sign in so it can be charged to your credits, or send an x402 payment")
		}
		canProceed, _, cost, err := quota.CheckQuota(sess.Account, op)
		return canProceed, cost, err
	}

	apps.QuotaCheck = agent.QuotaCheck

	// Deduct credits from the acting user for a metered call (SDK or the agent).
	chargeUser := func(r *http.Request, op string) {
		if sess, err := auth.GetSession(r); err == nil {
			_ = quota.Charge(sess.Account, op, nil)
		}
	}
	apps.ChargeQuota = chargeUser
	apps.ChargeUse = account.ChargeAppUse
	agent.ChargeQuota = chargeUser

	// Inline visual cards now come from the capability registry (core), which
	// each service self-registers into from its Load(). No central wiring here.

	// A paid wallet is an identity: an agent that has settled a payment can
	// reach account-scoped tools without an account. Read from the settled
	// payment only — never from the unauthenticated X-Wallet-Address header.
	api.WalletPayer = func(r *http.Request) string { return x402.PayerFrom(r.Context()) }
	// Read from the context rather than re-verified: the signature is checked
	// once at the door, because its nonce is single-use.
	api.WalletSigner = func(r *http.Request) string { return wallet.SignerFrom(r.Context()) }

	// Wire x402 payment required response for MCP
	if x402.Enabled() {
	}

	// Wire email sending for verification mails. Uses the platform's own
	// SMTP relay so verification mails come from no-reply@<MAIL_DOMAIN>.
	// Only enabled when MAIL_DOMAIN is configured to a real domain —
	// instances without mail configured skip the verification gate
	// entirely (see auth.VerificationRequired below).
	if domain := mail.ConfiguredDomain(); domain != "" && domain != "localhost" {
		app.EmailSender = func(to, subject, plain, html string) error {
			from := "no-reply@" + domain
			_, err := mail.SendExternalEmail("Mu", from, to, subject, plain, html, "")
			return err
		}
	}

	// Verification is only required when we can actually send verification
	// emails. Self-hosted instances without mail configured fall back to
	// the legacy "any account can post" rule.
	auth.VerificationRequired = func() bool {
		return app.EmailSender != nil
	}

	// Money is a trust signal, so auth needs to be able to see it. Wired as a
	// hook because the wallet imports auth and cannot be imported back.
	auth.HasCredit = func(accountID string) bool {
		return account.Balance(accountID) > 0
	}

	// The status page asks the AI package what the model is doing rather than
	// guessing from one env var.
	app.LLMStatus = ai.Status
	app.AgentStatus = agent.Status

	// Signup and login are not tools. Creating an account and exchanging
	// credentials for a session are how a caller comes to exist, not something
	// an existing caller can be granted — they live on the HTTP boundary (the
	// login form, /session) and in the CLI. An agent never needs them: it
	// authenticates with a token a human issued, or pays per request over x402,
	// where there is no account to create.
}

// localNow is the date and time where the account is, falling back to UTC.
//
// An agent that knows the city and not the hour still cannot answer "today":
// the model has a training cutoff and no clock, so without this line every
// question with a "now" in it is a guess. The zone comes from the browser when
// somebody sets their place — see account/place.go.
func localNow(accountID string) string {
	loc := time.UTC
	if acc, err := auth.GetAccount(accountID); err == nil && acc != nil && acc.Zone != "" {
		if z, err := time.LoadLocation(acc.Zone); err == nil {
			loc = z
		}
	}
	return time.Now().In(loc).Format("Monday 2 January 2006, 15:04 MST")
}
