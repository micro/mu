package server

// Bringing the instance up: the runtime core, then every service's own Load.
//
// Order matters in one place only — service.Init first, so a domain service can
// register itself as its package loads. The rest is independent.

import (
	"mu/account"
	"mu/admin"
	"mu/internal/data"
	"mu/internal/imageproxy"
	"mu/internal/service"
	"mu/internal/settings"
	"mu/internal/usage"
	"mu/internal/user"
	"mu/service/apps"
	"mu/service/archive"
	"mu/service/blog"
	"mu/service/bookmarks"
	"mu/service/browser"
	"mu/service/chat"
	"mu/service/contacts"
	"mu/service/docs"
	"mu/service/events"
	"mu/service/files"
	"mu/service/flights"
	"mu/service/food"
	"mu/service/hazards"
	"mu/service/images"
	"mu/service/mail"
	"mu/service/maps"
	"mu/service/markets"
	"mu/service/news"
	"mu/service/notes"
	"mu/service/notify"
	"mu/service/places"
	"mu/service/prayer"
	"mu/service/recall"
	"mu/service/routes"
	"mu/service/shell"
	"mu/service/sms"
	"mu/service/social"
	"mu/service/stream"
	"mu/service/tasks"
	"mu/service/text"
	"mu/service/transit"
	"mu/service/users"
	"mu/service/video"
	"mu/service/wallet"
	"mu/service/weather"
	"mu/service/web"
)

// boot starts the runtime core and loads every service.
func boot() {
	startupStep("service.Init", service.Init)

	// load settings first so other packages can use them
	startupStep("settings.Load", settings.Load)

	// load the data index
	startupStep("data.Load", data.Load)

	// Subscribe before services start publishing their first refresh.
	startupStep("stream.Load", stream.Load)

	// load admin/flags
	startupStep("admin.Load", admin.Load)

	// load the chat
	startupStep("chat.Load", chat.Load)

	// load the news
	startupStep("news.Load", news.Load)
	startupStep("news.StartSentimentLoop", news.StartSentimentLoop)

	// load the videos
	startupStep("video.Load", video.Load)

	// load the blog
	startupStep("blog.Load", blog.Load)

	// load the mail (also configures SMTP and DKIM)
	startupStep("mail.Load", mail.Load)

	// load places
	startupStep("places.Load", places.Load)

	// load weather
	startupStep("weather.Load", weather.Load)

	// load markets, reminder, wallet
	startupStep("markets.Load", markets.Load)
	startupStep("prayer.Load", prayer.Load)
	// Going looking in your own past on purpose — the read over internal/thread
	// that every client writes to. See service/recall.
	startupStep("recall.Load", recall.Load)
	startupStep("bookmarks.Load", bookmarks.Load)
	// One search across everything this instance has collected. Six services
	// write to that index and every reader over it was filtered to one type.
	startupStep("archive.Load", archive.Load)
	startupStep("browser.Load", browser.Load)
	startupStep("shell.Load", shell.Load)

	// And the SSH door onto it, when an operator has named a port. Off
	// otherwise — see service/sandbox/ssh.go for why the port is a decision
	// rather than a default.
	startupStep("shell.LoadSSH", shell.LoadSSH)
	startupStep("web.Load", web.Load)
	startupStep("text.Load", text.Load)
	startupStep("food.Load", food.Load)
	startupStep("transit.Load", transit.Load)
	startupStep("hazards.Load", hazards.Load)
	startupStep("maps.Load", maps.Load)
	startupStep("wallet.Load", wallet.Load)
	startupStep("stream.LoadService", stream.LoadService)
	startupStep("chat.LoadService", chat.LoadService)
	startupStep("docs.LoadService", docs.LoadService)
	startupStep("notes.LoadService", notes.LoadService)
	startupStep("notify.LoadService", notify.LoadService)
	startupStep("sms.LoadService", sms.LoadService)
	startupStep("images.Load", images.Load)
	// The cache behind /img, which serves article images from here instead of
	// from four publisher CDNs. See internal/imageproxy.
	startupStep("imageproxy.Load", imageproxy.Load)
	// Counters behind /admin/traffic: what this instance is being asked to do.
	startupStep("usage.Load", usage.Load)
	startupStep("files.Load", files.Load)

	// load flights
	startupStep("flights.Load", flights.Load)
	startupStep("routes.Load", routes.Load)
	startupStep("contacts.Load", contacts.Load)
	startupStep("users.Load", users.Load)
	// Who is here: the presence broadcaster behind /presence. It was started
	// from wireHooks, which is for breaking cycles rather than standing things
	// up, and it never needed one.
	startupStep("user.Load", user.Load)

	// These three loaded from wireHooks, which is for breaking cycles rather
	// than for standing services up. Nothing about them needed a hook first —
	// they were simply written where somebody was working — and the cost was
	// invisible until a test asked which Specs exist after boot and got an
	// answer three short. Loading is boot's job.
	startupStep("apps.Load", apps.Load)
	startupStep("social.Load", social.Load)
	startupStep("account.Load", account.Load)
	startupStep("tasks.Load", tasks.Load)
	startupStep("events.Load", events.Load)
}
