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

	"mu/admin"
	"mu/agent"
	agentblog "mu/agent/blog"
	"mu/agent/micro"
	agentsocial "mu/agent/social"
	"mu/client/discord"
	"mu/client/telegram"
	"mu/client/whatsapp"
	help "mu/docs"
	"mu/home"
	"mu/internal/ai"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/google"
	"mu/internal/notes"
	"mu/internal/profile"
	"mu/internal/quota"
	"mu/internal/service"
	"mu/internal/settings"
	"mu/service/apps"
	"mu/service/blog"
	"mu/service/contacts"
	"mu/service/docs"
	"mu/service/email"
	"mu/service/events"
	"mu/service/files"
	"mu/service/images"
	"mu/service/mail"
	"mu/service/markets"
	"mu/service/news"
	"mu/service/news/digest"
	"mu/service/sms"
	"mu/service/social"
	"mu/service/stream"
	"mu/service/tasks"
	"mu/service/user"
	"mu/service/web"
	whatsappsvc "mu/service/whatsapp"
	"mu/wallet"
)

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
		// The console is the instance's own timeline: what happened here, in
		// order — and nothing about whose it was or what it said.
		//
		// It used to post the reminder's title and the owner's account id. The
		// console is public: /stream serves it with no session, and stream_list
		// answers an unauthenticated MCP caller. So a reminder called "Dentist
		// about the biopsy results" was published to the open internet the
		// moment it fired, by the person who had written it down privately.
		// Anything carrying a title, a subject, a sender or an account id is
		// somebody's content and does not belong on a public timeline.
		stream.PostSystem("⏰ A reminder fired", map[string]any{
			"kind": stream.TypeReminder,
		})
		msg := "⏰ Event: " + title
		if note != "" {
			msg += "\n" + note
		}
		discord.NotifyUser(accountID, msg)
		telegram.NotifyUser(accountID, msg)
		whatsapp.NotifyUser(accountID, msg)
	}
	// A standing instruction: when an event carrying a prompt comes due, run it
	// through the agent and deliver the answer.
	//
	// This is the thing a server that stays up can do and a stdio MCP process
	// cannot — "every morning, brief me and mail it" has nowhere to live in a
	// process that only exists while a client is attached. Every piece was
	// already here (a scheduler, an agent, an inbox); nothing joined them.
	//
	// It runs in a goroutine because the agent may take tens of seconds and the
	// scheduler fires every event due in the same pass; one slow briefing must
	// not hold up the rest.
	// A task handed to the agent reaches it the same way a standing instruction
	// does: through a hook, so tasks does not import the agent.
	// A task run reports what it did; a scheduled one does not need to, and
	// keeps the simpler hook.
	tasks.RunAgent = func(accountID, prompt string, onStep func(tasks.Step)) (string, error) {
		opts := agent.QueryOpts{}
		if onStep != nil {
			opts.OnStep = func(s agent.Step) {
				onStep(tasks.Step{
					Tool:    s.Tool,
					Detail:  tasks.StepDetail(s.Args),
					OK:      s.OK,
					Seconds: s.Took.Seconds(),
				})
			}
		}
		return agent.QueryWithOpts(accountID, prompt, opts)
	}
	events.RunAgent = func(accountID, prompt string) (string, error) {
		return agent.Query(accountID, prompt)
	}

	events.OnFireEvent = func(e *events.Event) {
		if strings.TrimSpace(e.Prompt) == "" {
			return
		}
		go func(e events.Event) {
			// Charged like any other agent run — see service/events/run.go.
			// Always returns something to deliver, including the reason when it
			// could not run at all.
			answer := events.RunPrompt(&e)

			// Mail is the delivery that survives being away from the screen,
			// and this instance runs the inbox. Tagged so an agent can read
			// back only its own scheduled results.
			if acc, err := auth.GetAccount(e.Owner); err == nil {
				_ = mail.SendMessageTo("Mu", "agent@"+mail.GetConfiguredDomain(),
					acc.Name, acc.ID, "scheduled", e.Title, answer, "", "", false, 0, nil, "", "", nil)
			}
			// Contentless, for the reason on events.OnFire above: the title
			// of a standing instruction is as private as the reminder.
			stream.PostSystem("⏰ A scheduled instruction ran", map[string]any{
				"kind": stream.TypeReminder,
			})
		}(*e)
	}

	// Mail addressed to an agent wakes it, and it answers in the thread.
	//
	// Every agent already had an address. Writing to one filed a message in the
	// owner's inbox and nothing else — an agent with an address that cannot
	// answer is a mailbox with a name on it, and emailing your agent is the
	// first thing anyone tries with one.
	//
	// It answers as that agent: its standing instruction and its scope, so a
	// research agent you emailed cannot read your mail unless you gave it mail.
	// Charged like any other agent run, checked before the model is asked so a
	// run that cannot be paid for does not spend one first — and the sender is
	// told, because silence is what this looked like before.
	// Who may wake one. The sender has to pass SPF or DKIM and be somebody this
	// account knows — its own verified address, checked inside mail, or a name
	// in its address book, which is this hook because contacts is a different
	// domain and mail should not import it.
	mail.KnownSender = func(owner, addr string) bool {
		return contacts.HasEmail(owner, addr)
	}

	mail.InboundAgent = func(m mail.InboundMail) {
		// Which agent answers. A tag names one; the shared address names none,
		// so the account's default assistant takes it — the thing you get when
		// you have not made an agent yet, which is most people writing to
		// agent@ for the first time.
		var a *agent.Agent
		if !m.Shared {
			if a = agent.AgentForTag(m.Owner, m.Tag); a == nil {
				return // a tag that is not an agent: ordinary tagged mail
			}
		}
		name, ref := "Micro", ""
		if a != nil {
			name, ref = a.Name, a.ID
		}
		go func() {
			domain := mail.GetConfiguredDomain()
			// Reply from the address that reaches this agent again, so hitting
			// reply continues the conversation. It used to answer from
			// agent@<domain> whoever had written to, which was a dead letter
			// until that address started resolving, and still loses which
			// agent you were talking to.
			from := mail.SharedAgentAddress()
			if a != nil && a.Address() != "" {
				from = a.Address()
			}
			reply := func(body string) {
				if domain == "" || domain == "localhost" || from == "" {
					return
				}
				subject := m.Subject
				if !strings.HasPrefix(strings.ToLower(subject), "re:") {
					subject = "Re: " + subject
				}
				if _, err := mail.SendExternalEmail(name, from, m.From, subject, body, "", m.MessageID); err != nil {
					app.Log("mail", "agent %s could not reply to %s: %v", name, m.From, err)
				}
			}

			canProceed, _, cost, err := quota.CheckQuota(m.Owner, quota.OpAgentQuery)
			if err != nil || !canProceed {
				reply(fmt.Sprintf("I could not run this one: it costs %d credits and the account is short. "+
					"Top up at %s/wallet and send it again.", cost, app.PublicURL()))
				return
			}

			opts, err := agent.AskAs(m.Owner, ref)
			if err != nil {
				app.Log("mail", "agent %s could not be resolved: %v", name, err)
				return
			}
			prompt := m.Subject
			if body := strings.TrimSpace(m.Body); body != "" {
				if prompt != "" {
					prompt += "\n\n"
				}
				prompt += body
			}
			if strings.TrimSpace(prompt) == "" {
				return
			}

			answer, err := agent.QueryWithOpts(m.Owner, prompt, opts)
			if err != nil {
				app.Log("mail", "agent %s failed on mail from %s: %v", name, m.From, err)
				reply("I could not answer that one. Try again, or ask a different way.")
				return
			}
			reply(answer)
		}()
	}

	// When an event is scheduled, email the owner an .ics invite so it also
	// lands in their real calendar. Only for users with a verified email (e.g.
	// via Google sign-in) and only when this instance can send mail.
	events.OnCreate = func(e *events.Event) {
		domain := mail.GetConfiguredDomain()
		if domain == "" || domain == "localhost" {
			return
		}
		acc, err := auth.GetAccount(e.Owner)
		if err != nil || acc.Email == "" || !acc.EmailVerified {
			return
		}
		when := e.When.Local().Format("Mon 2 Jan 2006, 15:04 MST")
		body := fmt.Sprintf(`<p>You asked Mu to remind you:</p><p style="font-size:16px"><strong>%s</strong><br>%s</p>`,
			html.EscapeString(e.Title), html.EscapeString(when))
		if e.Note != "" {
			body += `<p>` + html.EscapeString(e.Note) + `</p>`
		}
		body += `<p style="color:#888;font-size:13px">Added to your calendar from the attached invite.</p>`
		ics := events.ICS(e, acc.Email)
		if _, err := mail.SendCalendarInvite("Mu Events", "no-reply@"+domain, acc.Email, "Event: "+e.Title, body, ics); err != nil {
			app.Log("events", "calendar invite to %s failed: %v", acc.Email, err)
		}
	}
	app.LinkCodeFunc = auth.GenerateLinkCode
	app.ToolCountFunc = api.ToolCount

	discord.Load()
	telegram.Load()
	whatsapp.Load()
	mail.OnNewMail = func(accountID, from, subject, body string) {
		// The worst of the three: this published who wrote to somebody and
		// what about, to a timeline anybody can read without signing in. The
		// notifications below go to the owner's own linked channels, which is
		// where a sender and a subject belong.
		stream.PostSystem("📬 Mail arrived", map[string]any{
			"kind": stream.TypeSystem,
		})
		summary := discord.SummariseEmail(from, subject, body)
		discord.NotifyNewMail(accountID, from, subject, summary)
		telegram.NotifyUser(accountID, fmt.Sprintf("📬 *New email from %s*\n%s", from, summary))
		whatsapp.NotifyUser(accountID, fmt.Sprintf("📬 *New email from %s*\n%s", from, summary))
	}

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

	// load the home cards
	home.Load()

	// load agent
	agent.Load()

	// Wire user context into the agent — personalises responses.
	userCtxFunc := func(accountID string) string {
		var parts []string
		// Unread mail count.
		if unread := mail.GetUnreadCount(accountID); unread > 0 {
			parts = append(parts, fmt.Sprintf("- %d unread email(s)", unread))
		}
		// Wallet balance.
		bal := wallet.GetBalance(accountID)
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
	micro.UserContextFunc = userCtxFunc

	// Wire digest → blog callbacks (digest publishes as blog post)
	digest.PublishBlogPost = func(title, content, author, authorID, tags string) (string, error) {
		err := blog.CreatePost(title, content, author, authorID, tags, false)
		if err != nil {
			return "", err
		}
		// Return the ID of the just-created post
		post := blog.FindTodayDigest()
		if post != nil {
			return post.ID, nil
		}
		return "", nil
	}
	digest.UpdateBlogPost = func(id, title, content, tags string) error {
		return blog.UpdatePost(id, title, content, tags, false)
	}
	digest.FindTodayBlogDigest = func() *digest.DigestPost {
		post := blog.FindTodayDigest()
		if post == nil {
			return nil
		}
		return &digest.DigestPost{
			ID:      post.ID,
			Title:   post.Title,
			Content: post.Content,
		}
	}

	// load daily digest scheduler
	digest.Load()

	// load search
	web.Load()

	// load docs
	help.Load()

	// load user presence tracking
	profile.Load()

	// Load the stream (platform event timeline).
	stream.Load()

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
	profile.GetUserPosts = func(authorID, authorName string) []profile.UserPost {
		posts := blog.GetPostsByAuthorID(authorID, authorName)
		result := make([]profile.UserPost, len(posts))
		for i, p := range posts {
			result[i] = profile.UserPost{
				ID:        p.ID,
				Title:     p.Title,
				Content:   p.Content,
				CreatedAt: p.CreatedAt,
				Private:   p.Private,
			}
		}
		return result
	}
	profile.LinkifyContent = blog.Linkify

	// Wire @micro replies in the stream: run the agent against the sender's
	// wallet and post the answer back into the timeline. Async, so the POST
	// returns immediately. The same hook existed for statuses; the stream is
	// the timeline that survived.
	stream.AIReplyHook = func(askerID, prompt string) {
		if auth.IsBanned(askerID) {
			return
		}
		answer, err := agent.Query(askerID, prompt)
		if err != nil {
			app.Log("stream", "@micro agent error for %s: %v", askerID, err)
			stream.PostAgent("I couldn't answer that one — try again in a moment.")
			return
		}
		answer = strings.TrimSpace(answer)
		if answer == "" {
			return
		}
		if !profile.ModerateAIResponse(askerID, answer) {
			app.Log("stream", "AI response for %s blocked by moderation", askerID)
			return
		}
		stream.PostAgent(answer)
	}

	profile.GetUserApps = func(authorID string) []profile.UserApp {
		appList := apps.GetAppsByAuthor(authorID)
		result := make([]profile.UserApp, len(appList))
		for i, a := range appList {
			result[i] = profile.UserApp{
				Slug:        a.Slug,
				Name:        a.Name,
				Description: a.Description,
				Icon:        a.Icon,
			}
		}
		return result
	}

	// Wire admin → blog callbacks (avoids blog importing admin)
	admin.GetNewAccountBlog = blog.GetNewAccountBlogPosts
	admin.RefreshBlogCache = blog.RefreshCache

	// Register account deletion hooks — each package cleans up its own data.
	auth.AccountDeleteHooks = append(auth.AccountDeleteHooks,
		blog.DeletePostsByAuthor,
		social.DeleteByAuthor,
		apps.DeleteAppsByAuthor,
		stream.ClearByAuthor,
		mail.DeleteInbox,
		func(id string) { wallet.DeleteWallet(id) },
		func(id string) { wallet.DeleteBaseWallet(id) },
		func(id string) { micro.DeleteUserAgents(id) },
		func(id string) { discord.DeleteLinks(id) },
		func(id string) { telegram.DeleteLinks(id) },
		func(id string) { whatsapp.DeleteLinks(id) },
		func(id string) { user.Delete(id) },
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
		email.DeleteAll,
		whatsappsvc.DeleteAll,
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

		// A free allowance is spent before a credit is. Checking it here rather
		// than inside CheckQuota keeps the two ideas apart: what an operation
		// costs is arithmetic about providers, and how much of it somebody gets
		// for nothing is an operator's commercial choice.
		if quota.WithinAllowance(account, op) {
			// Inside the allowance: allowed, recorded, not charged. Recorded
			// because /usage should show what an account actually did, and a
			// free call is still something it did.
			quota.Record(account, op)
			return false, nil
		}
		ok, _, cost, err := quota.CheckQuota(account, op)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, fmt.Errorf("this costs %d credits and your balance is %d — top up at /wallet",
				cost, quota.BalanceOf(account))
		}
		return true, nil
	}
	service.Gate.Charge = func(account, op string) {
		if err := quota.ConsumeWith(account, op, nil); err != nil {
			app.Log("wallet", "charging %s for %s: %v", account, op, err)
		}
	}
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
	email.SendVia = mail.SendExternalAs

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
		if r.Context().Value(wallet.X402ContextKey) != nil {
			// Try free trial first (by wallet address from payment header).
			payAddr := r.Header.Get("X-Wallet-Address")
			if payAddr != "" && wallet.X402UseTrialCall(payAddr) {
				return true, 0, nil
			}
			_, err := wallet.VerifyAndSettle(r, op, r.URL.Path)
			if err != nil {
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
		if r.Context().Value(wallet.X402ContextKey) != nil {
			_, err := wallet.VerifyAndSettle(r, op, r.URL.Path)
			if err != nil {
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
			_ = quota.ConsumeQuota(sess.Account, op)
		}
	}
	apps.ChargeQuota = chargeUser
	apps.ChargeUse = wallet.ChargeAppUse
	agent.ChargeQuota = chargeUser

	// Inline visual cards now come from the capability registry (core), which
	// each service self-registers into from its Load(). No central wiring here.

	// A paid wallet is an identity: an agent that has settled a payment can
	// reach account-scoped tools without an account. Read from the settled
	// payment only — never from the unauthenticated X-Wallet-Address header.
	api.WalletPayer = func(r *http.Request) string { return wallet.PayerFrom(r.Context()) }

	// Wire x402 payment required response for MCP
	if wallet.X402Enabled() {
	}

	// Wire email sending for verification mails. Uses the platform's own
	// SMTP relay so verification mails come from no-reply@<MAIL_DOMAIN>.
	// Only enabled when MAIL_DOMAIN is configured to a real domain —
	// instances without mail configured skip the verification gate
	// entirely (see auth.VerificationRequired below).
	if domain := mail.GetConfiguredDomain(); domain != "" && domain != "localhost" {
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
		return wallet.GetBalance(accountID) > 0
	}

	// The status page asks the AI package what the model is doing rather than
	// guessing from one env var.
	app.LLMStatus = ai.Status

	// Signup and login are not tools. Creating an account and exchanging
	// credentials for a session are how a caller comes to exist, not something
	// an existing caller can be granted — they live on the HTTP boundary (the
	// login form, /session) and in the CLI. An agent never needs them: it
	// authenticates with a token a human issued, or pays per request over x402,
	// where there is no account to create.
}
