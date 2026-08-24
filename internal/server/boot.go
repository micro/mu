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
	"mu/service/video"
	"mu/service/wallet"
	"mu/service/weather"
	"mu/service/web"
)

// boot starts the runtime core and loads every service.
func boot() {
	service.Init()

	// load settings first so other packages can use them
	settings.Load()

	// load the data index
	data.Load()

	// load admin/flags
	admin.Load()

	// load the chat
	chat.Load()

	// load the news
	news.Load()
	news.StartSentimentLoop()

	// load the videos
	video.Load()

	// load the blog
	blog.Load()

	// load the mail (also configures SMTP and DKIM)
	mail.Load()

	// load places
	places.Load()

	// load weather
	weather.Load()

	// load markets, reminder, wallet
	markets.Load()
	prayer.Load()
	// Going looking in your own past on purpose — the read over internal/thread
	// that every client writes to. See service/recall.
	recall.Load()
	// One search across everything this instance has collected. Six services
	// write to that index and every reader over it was filtered to one type.
	archive.Load()
	browser.Load()
	shell.Load()

	// And the SSH door onto it, when an operator has named a port. Off
	// otherwise — see service/sandbox/ssh.go for why the port is a decision
	// rather than a default.
	shell.LoadSSH()
	web.Load()
	text.Load()
	food.Load()
	transit.Load()
	hazards.Load()
	maps.Load()
	wallet.Load()
	stream.LoadService()
	chat.LoadService()
	docs.LoadService()
	notes.LoadService()
	sms.LoadService()
	images.Load()
	// The cache behind /img, which serves article images from here instead of
	// from four publisher CDNs. See internal/imageproxy.
	imageproxy.Load()
	// Counters behind /admin/traffic: what this instance is being asked to do.
	usage.Load()
	files.Load()

	// load flights
	flights.Load()
	routes.Load()
	contacts.Load()
	// Who is here: the presence broadcaster behind /presence. It was started
	// from wireHooks, which is for breaking cycles rather than standing things
	// up, and it never needed one.
	user.Load()

	// These three loaded from wireHooks, which is for breaking cycles rather
	// than for standing services up. Nothing about them needed a hook first —
	// they were simply written where somebody was working — and the cost was
	// invisible until a test asked which Specs exist after boot and got an
	// answer three short. Loading is boot's job.
	apps.Load()
	social.Load()
	account.Load()
	tasks.Load()
	events.Load()
}
