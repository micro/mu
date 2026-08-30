// Package arrivals is what came in today, counted from the archive.
//
// The services fetch all day — news, video, the social firehose — and index
// what they fetch. Nothing reads it back. The archive is searchable if an agent
// chooses to call archive_search, and the cards on the front page show the
// newest few of each kind, but there is nowhere that says how much arrived,
// which is the one question somebody has before they decide whether to look.
//
// So this counts. Not a summary and not a judgement: how many stories, how many
// videos, how many posts, and the title of the most recent. home/brief.go turns
// that into a clause beside the ones about your mail and your tasks, which is
// where the answer belongs — one line, before you look anywhere.
//
// # Why nothing calls a model
//
// The same reason home/brief.go gives: this is on the screen somebody sees most
// often, and a model call per page view costs a credit and two seconds to say
// something four numbers already say. A model pass over these facts is the next
// thing to add — once a day, saying what is worth attention rather than what
// exists — and the shape here is built for it: Today() already decides when the
// answer has changed, which is the only part of that which is hard.
//
// # Why the answer is not stored
//
// Every fact here is derived, and rebuilding it costs three row reads. There is
// nothing to lose across a restart, so a JSON file would be a migration and a
// deletion hook in exchange for nothing. That changes the day a model writes
// the sentence: a paid-for judgement is not derivable and has to survive a
// restart. The cache below is where it goes, and that is the whole diff.
package arrivals

import (
	"sync"
	"time"

	"mu/internal/data"
)

// Kind is one stream of arrivals, with the words to count it in.
type Kind struct {
	// Type is the archive row's type, as the fetching service indexes it.
	Type string
	// One and Many are what to call the things. Not the type name: the rows
	// are typed "social" and a person has never received nine socials.
	One, Many string
	// Headline is whether the row's title names the thing, so the newest of
	// them can be read out. News and video rows are titled with a headline. A
	// social row is titled with whoever posted it — the title field is what
	// each service happened to put there, and "the newest “someone”" is not a
	// fact about anything.
	Headline bool
}

// Kinds are the streams worth telling somebody about, read out in this order.
//
// Three, and the omissions are deliberate.
//
// market is out because every ticker overwrites one row per refresh. "What is
// new" would fire on each poll and count the same thirty-four symbols again — a
// number that changes every fifteen minutes and carries nothing. A price is a
// level rather than an arrival, and saying something true about it needs the
// previous level, which nothing keeps yet.
//
// reminder is out because it lands hourly by construction, so it would be the
// largest number on the line every day and always the same one. What is in
// those rows matters, but it belongs in a reflection read against the day, not
// in a tally of how busy the wire was.
//
// post — the blog — is out because it is this instance writing, not the world
// arriving, and the daily digest would put a permanent "1 article" on the line.
var Kinds = []Kind{
	{Type: "news", One: "story", Many: "stories", Headline: true},
	{Type: "video", One: "video", Many: "videos", Headline: true},
	{Type: "social", One: "post", Many: "posts"},
}

// window caps how many rows of one kind are read to count a day.
//
// Every row carries its content, and a news row's content is the article, so an
// unbounded read to answer "how many" is megabytes to produce a number. A kind
// that lands more than this in a day is undercounted, which is a fair trade at
// the point where the honest answer is "more than you are going to read".
const window = 120

// stale is how long a cached answer is trusted when nothing has arrived.
//
// The gate below only notices things appearing, so without this a count can
// only ever go up: rows do get removed — an account deleted, a superseded
// store swept — and the front page would hold a number for things that are no
// longer there until the next arrival happened to correct it. Two minutes is
// what home's card cache uses, and the recount it forces is three row reads.
const stale = 2 * time.Minute

// Day is what arrived since local midnight.
type Day struct {
	// Counts is kind type → how many, absent when none.
	Counts map[string]int
	// Newest is the title of the most recent arrival, for saying what the
	// number is made of. A count is a number; a count and a headline is a
	// reason to look or a reason not to.
	Newest string
	// Day is the midnight this counts from, and At is when it was worked out.
	Day, At time.Time
}

// Total is how many things arrived, across every kind.
func (d Day) Total() int {
	n := 0
	for _, c := range d.Counts {
		n += c
	}
	return n
}

// Any is whether there is anything to say.
func (d Day) Any() bool { return d.Total() > 0 }

var (
	mu     sync.Mutex
	cached Day
	// seen is the newest arrival the cached answer was worked out from. It is
	// the whole invalidation: a count of today cannot change without something
	// arriving, and something arriving moves this.
	seen time.Time
)

// Today is what came in today, worked out again only when it has changed.
//
// The check is three rows — the newest of each kind — against what the cached
// answer already saw. It is monotonic, so it can fire when nothing really
// changed (a news row re-indexed with fuller content bumps its stamp) and the
// recount then produces the same answer, but it cannot miss an arrival. Being
// occasionally redundant is the right way round for a gate in front of a read
// this size.
func Today() Day {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	day := midnight(now)
	newest := latest()

	if cached.Day.Equal(day) && !newest.After(seen) && now.Sub(cached.At) < stale {
		return cached
	}

	cached = count(day)
	seen = newest
	return cached
}

// latest is the most recent arrival across the kinds, by when it was indexed.
//
// Indexed rather than posted, because this is asking whether the archive has
// moved, and it is the index stamp that moves. The count below asks a different
// question — did this happen today — and reads the posted stamp for it.
func latest() time.Time {
	var newest time.Time
	for _, k := range Kinds {
		for _, e := range data.ByType(k.Type, 1) {
			if e.IndexedAt.After(newest) {
				newest = e.IndexedAt
			}
		}
	}
	return newest
}

// count reads what arrived on or after a midnight.
//
// Filtered on when the thing happened, not when it was indexed: a week-old
// article discovered this morning is not something that happened today, and an
// article re-indexed this morning did not happen twice. data.PostedAt falls
// back to the index stamp for rows that never carried one, so a service that
// does not say when loses nothing.
func count(day time.Time) Day {
	out := Day{Counts: map[string]int{}, Day: day, At: time.Now()}
	var newestAt time.Time

	for _, k := range Kinds {
		for _, e := range data.ByType(k.Type, window) {
			at := data.PostedAt(e)
			if at.Before(day) {
				continue
			}
			out.Counts[k.Type]++
			if k.Headline && at.After(newestAt) {
				newestAt, out.Newest = at, e.Title
			}
		}
	}
	return out
}

// midnight is the start of the day a time falls in, in its own zone.
func midnight(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// forget drops the cached answer. For tests, which build an archive row by row
// and would otherwise read whatever the previous test left behind.
func forget() {
	mu.Lock()
	defer mu.Unlock()
	cached, seen = Day{}, time.Time{}
}
