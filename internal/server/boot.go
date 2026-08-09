package server

// Bringing the instance up: the runtime core, then every service's own Load.
//
// Order matters in one place only — service.Init first, so a domain service can
// register itself as its package loads. The rest is independent.

import (
	"mu/admin"
	"mu/internal/data"
	"mu/internal/imageproxy"
	"mu/internal/service"
	"mu/internal/settings"
	"mu/internal/usage"
	"mu/service/blog"
	"mu/service/chat"
	"mu/service/contacts"
	"mu/service/db"
	"mu/service/events"
	"mu/service/files"
	"mu/service/images"
	"mu/service/mail"
	"mu/service/markets"
	memsvc "mu/service/memory"
	"mu/service/news"
	"mu/service/places"
	"mu/service/prayer"
	"mu/service/stream"
	"mu/service/tasks"
	"mu/service/video"
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
	web.Load()
	stream.LoadService()
	chat.LoadService()
	db.LoadService()
	memsvc.LoadService()
	images.Load()
	// The cache behind /img, which serves article images from here instead of
	// from four publisher CDNs. See internal/imageproxy.
	imageproxy.Load()
	// Counters behind /admin/traffic: what this instance is being asked to do.
	usage.Load()
	files.Load()
	contacts.Load()
	tasks.Load()
	events.Load()
}
