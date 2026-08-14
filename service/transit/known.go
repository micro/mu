package transit

// Feeds worth switching on, and what each one costs.
//
// The catalogue holds about 1,160 keyless timetables and most of them are a
// single bus company in a town you have never heard of. This is the shortlist:
// every entry was downloaded and built, so the sizes are measured rather than
// guessed, and the ones that turned out to be broken or years out of date are
// listed as such instead of being quietly dropped.
//
// That last part is the point of writing it down. "Keyless" and "current" are
// not the same set — San Francisco's Muni, Paris's RATP, Sydney and Stockholm
// all publish keyless feeds whose timetables ran out, in one case in 2019. A
// list that only named the good ones would send somebody to rediscover that.
//
// Switch one on by putting its name in TRANSIT_FEEDS, comma separated.

// Known is a feed somebody might want, with what it costs to carry.
type Known struct {
	// Query is what to put in TRANSIT_FEEDS.
	Query   string
	Place   string
	Country string
	// MB is the download, measured. The index on disk is roughly eight bytes
	// per departure, which is usually smaller than the zip.
	MB   float64
	Note string
}

// Status is what was true when each was last checked by hand.
const (
	noteFrequency = "headway-based, so most departures are computed rather than listed"
	noteMirror    = "the agency's own address was dead; loads from the catalogue mirror"
	noteBig       = "large — expect a slow first build"
)

// KnownFeeds is the shortlist, roughly smallest first within each region.
//
// Nothing here is loaded automatically. A feed is tens of megabytes and an
// instance should carry the ones its people actually travel on.
var KnownFeeds = []Known{
	// ── United Kingdom ──────────────────────────────────────────────────
	{"reading buses", "Reading", "GB", 2.6, ""},
	{"nottingham city transport", "Nottingham", "GB", 3.2, ""},
	{"metrobus", "Gatwick, Crawley and Surrey", "GB", 4.0, ""},
	{"cardiff bus", "Cardiff", "GB", 4.4, ""},
	{"brighton & hove bus", "Brighton", "GB", 4.9, ""},
	{"east yorkshire", "East Yorkshire", "GB", 6.3, ""},
	{"transdev blazefield", "Yorkshire and Lancashire", "GB", 7.6, ""},
	{"oxford bus", "Oxford", "GB", 8.6, ""},
	{"go cornwall bus", "Cornwall", "GB", 9.0, ""},
	{"go north east", "Tyne and Wear", "GB", 12.3, ""},
	{"chiltern railways", "London Marylebone to Birmingham", "GB", 18.6, noteMirror + "; the only railway here"},
	{"intalink", "Hertfordshire", "GB", 23.1, "every operator in the county"},
	{"essex county council", "Essex", "GB", 34.8, noteMirror},
	{"transport for greater manchester", "Greater Manchester", "GB", 46.0, noteBig + "; covers about forty operators"},

	// ── Elsewhere ───────────────────────────────────────────────────────
	{"bay area rapid transit", "San Francisco Bay Area", "US", 0.9, "trains only; Muni's own feed has expired"},
	{"metropolitan transit authority", "New York", "US", 6.4, "the subway, and the best value on this list"},
	{"smrt", "Singapore", "SG", 6.7, noteFrequency},
	{"àrea metropolitana de barcelona", "Barcelona", "ES", 7.4, ""},
	{"empresa municipal de transportes", "Madrid", "ES", 16.8, noteFrequency},
	{"roma servizi per la mobilità", "Rome", "IT", 36.7, ""},
	{"carris", "Lisbon", "PT", 45.8, ""},
	{"helsingin seudun liikenne", "Helsinki", "FI", 52.6, noteBig},
	{"warszawski transport publiczny", "Warsaw", "PL", 68.4, noteBig + "; " + noteFrequency},
	{"wiener lokalbahnen", "Vienna", "AT", 78.2, noteBig},
	{"verkehrsverbund berlin-brandenburg", "Berlin and Brandenburg", "DE", 75.5, noteBig},
	{"île-de-france mobilités", "Paris", "FR", 167.0, "enormous — 15 million departures, a 114MB index"},
}

// Expired are feeds that look right and are not.
//
// Kept deliberately: each is the obvious thing to reach for, and each would be
// refused after the download because its timetable has run out. Naming them
// saves somebody the trip.
var Expired = []Known{
	{"san francisco municipal transportation agency", "San Francisco", "US", 12.4, "ran out August 2024"},
	{"toronto transit commission", "Toronto", "CA", 27.2, "ran out June 2026"},
	{"régie autonome des transports parisiens", "Paris", "FR", 76.1, "ran out September 2021 — use Île-de-France Mobilités"},
	{"transport for new south wales", "Sydney", "AU", 133.3, "ran out March 2019"},
	{"trafiklab", "Stockholm", "SE", 49.3, "ran out December 2020"},
	{"port authority trans-hudson", "New York and New Jersey", "US", 1.2, "ran out June 2026"},
}
