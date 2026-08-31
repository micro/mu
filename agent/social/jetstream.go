package social

// Watching the open social network for things worth surfacing.
//
// Social had no source. It was meant to be a public timeline and the only thing
// that ever reached it was this instance's own news, matched against itself —
// a story reported under two categories was called breaking, which is a guess
// about editorial attention rather than a signal from anybody.
//
// ATProto gives it one. Jetstream is Bluesky's firehose as plain JSON over a
// websocket: no key, no account, no rate-limit negotiation, which is the same
// property that makes the rest of this instance worth having. Measured before
// building: about 39 posts a second across the network, 41% of them English,
// half of those replies, a quarter carrying a link.
//
// Which is the whole design problem. Three million posts a day is not a feed,
// it is weather. Everything below is about getting from that to a handful worth
// reading, and the order matters: refuse the obvious noise cheaply, score what
// survives, and surface only the best of a batch rather than everything that
// passes.
//
// Off unless an operator turns it on. Every other default in Mu is "works with
// no configuration", and this one is not: pulling strangers' posts into an
// instance is a decision about what the operator is willing to publish, and it
// is theirs to make rather than ours to assume.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gorilla/websocket"

	"mu/internal/app"
	"mu/internal/settings"
)

// jetstreamHost is the public Jetstream endpoint. A variable so a test can
// point it at a stub: a test that reaches the real firehose is slow, flaky and
// rude to somebody's free service.
var jetstreamHost = "wss://jetstream1.us-east.bsky.network/subscribe"

const (
	// reviewEvery is how often the collected candidates are judged. Long enough
	// that there is a batch to choose the best of; short enough that "breaking"
	// still means something.
	reviewEvery = 15 * time.Minute

	// sampleFor is how long the connection stays open inside that window.
	//
	// This is the whole cost story. Measured on the live firehose: 2172 messages
	// and 1.8 MB a minute, which held open is 2.6 GB a day — to find eleven
	// candidates a minute, of which three are published every fifteen. Holding
	// the connection open the other thirteen and a half minutes buys nothing but
	// a bigger buffer to throw away.
	//
	// Bluesky's own levers were measured and rejected. compress=true is real
	// — 58% fewer bytes — but the stream needs their custom zstd dictionary, a
	// binary blob to vendor and keep in step with theirs. maxMessageSizeBytes
	// works and saves between 1% and 7%, and every threshold that saved anything
	// also dropped a candidate. Neither is worth having next to a 90% cut that
	// costs nothing and is kinder to somebody giving this away free.
	sampleFor = 90 * time.Second

	// surfacePerReview caps what one review can post. The filters below pass
	// something like a post a minute; without this, social would become a wall
	// of other people's links, which is the thing it is trying not to be.
	surfacePerReview = 3

	// maxCandidates bounds the buffer between reviews. A firehose must never be
	// able to grow this instance's memory without limit.
	maxCandidates = 400

	// minText is how short a post can be and still be worth reading somewhere
	// else. Below this it is a reaction — "this", "lol", "same" — which means
	// nothing without the thing it is reacting to.
	minText = 80

	// maxTags is a spam signal. A post carrying six hashtags is addressed to a
	// search index rather than to a person.
	maxTags = 5

	// latinShare is how much of a post's lettering must be in the Latin script
	// for it to pass as English. Not 1: an English post can quote a name or a
	// word in another script and still be English. Low enough that a sentence
	// of Japanese with an English hashtag on the end does not pass.
	latinShare = 0.85
)

// Enabled reports whether this instance watches the network.
func Enabled() bool {
	v := strings.ToLower(strings.TrimSpace(settings.Get("SOCIAL_ATPROTO")))
	return v == "1" || v == "true" || v == "yes"
}

// candidate is one post that survived the filters, with what we know about why.
type candidate struct {
	DID      string
	Text     string
	Link     string
	Category string
	Media    bool
	Score    int
	At       time.Time
}

var (
	mu         sync.Mutex
	candidates []*candidate
	surfaced   = map[string]bool{}
)

// Watch samples the network on a schedule: listen for a window, hang up, decide
// what is worth publishing, wait.
//
// There is no cursor. A cursor exists to close the gap a restart leaves, and
// this deliberately leaves a gap thirteen minutes wide — the point is not to
// see everything the network says, it is to find three things worth reading,
// and those are abundant. Asking Jetstream to replay what we skipped would undo
// the only saving that matters.
func Watch() {
	if !Enabled() {
		app.Log("social", "atproto: off (set SOCIAL_ATPROTO=true to watch the network)")
		return
	}
	for {
		if _, _, err := sample(); err != nil {
			// No backoff ladder: the next window is already minutes away, which
			// is a politer retry than any ladder would pick.
			app.Log("social", "atproto: %v", err)
		}
		review()
		time.Sleep(reviewEvery - sampleFor)
	}
}

// sample listens for one window and returns how much went past and how much
// was kept.
func sample() (seen, kept int, err error) {
	conn, _, err := websocket.DefaultDialer.Dial(
		jetstreamHost+"?wantedCollections=app.bsky.feed.post", nil)
	if err != nil {
		return 0, 0, fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(sampleFor)) //nolint:errcheck
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break // the read deadline, normally: the window is over
		}
		seen++
		if !worthParsing(msg) {
			continue
		}
		var ev event
		if json.Unmarshal(msg, &ev) != nil {
			continue
		}
		if c := consider(ev); c != nil {
			keep(c)
			kept++
		}
	}
	app.Log("social", "atproto: %d posts in %s, %d kept", seen, sampleFor, kept)
	return seen, kept, nil
}

// ── Refusing before parsing ─────────────────────────────────────

var (
	markCreate = []byte(`"operation":"create"`)
	markReply  = []byte(`"reply":{`)
	markLangs  = []byte(`"langs":[`)
)

// worthParsing is the same refusal as the first three tests in consider, read
// off the raw bytes so the common case never reaches the JSON decoder.
//
// Measured over 2172 live messages: 134 were not creates, 1237 were not in
// English and 449 were replies — 84% of everything, decided by three fields.
// Unmarshalling all of that into a struct in order to throw it away is the
// CPU this loop was actually spending.
//
// It has to agree with consider or it is a silent filter nobody wrote:
// TestTheFastPathAgreesWithTheSlowOne holds them together.
func worthParsing(msg []byte) bool {
	if !bytes.Contains(msg, markCreate) || bytes.Contains(msg, markReply) {
		return false
	}
	i := bytes.Index(msg, markLangs)
	if i < 0 {
		return false // no language tag at all, so not one we can read
	}
	rest := msg[i+len(markLangs):]
	if end := bytes.IndexByte(rest, ']'); end >= 0 {
		rest = rest[:end]
	}
	return bytes.Contains(rest, []byte(`"en`))
}

// ── The event ───────────────────────────────────────────────────

type event struct {
	DID    string `json:"did"`
	TimeUS int64  `json:"time_us"`
	Kind   string `json:"kind"`
	Commit struct {
		Operation  string `json:"operation"`
		Collection string `json:"collection"`
		Record     record `json:"record"`
	} `json:"commit"`
}

type record struct {
	Text      string   `json:"text"`
	Langs     []string `json:"langs"`
	CreatedAt string   `json:"createdAt"`
	Reply     *struct {
		Root any `json:"root"`
	} `json:"reply"`
	Facets []struct {
		Features []struct {
			Type string `json:"$type"`
			URI  string `json:"uri"`
			Tag  string `json:"tag"`
		} `json:"features"`
	} `json:"facets"`
	Embed *struct {
		Type     string `json:"$type"`
		External *struct {
			URI         string `json:"uri"`
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"external"`
		Images []any `json:"images"`
		Video  any   `json:"video"`
	} `json:"embed"`
}

// ── The filter ──────────────────────────────────────────────────

// consider decides whether one event is worth keeping, cheapest tests first.
//
// Every rejection here is mechanical — a fact about the post, not a judgement
// about it. What is interesting is decided later, over a batch, where there is
// something to compare against.
func consider(ev event) *candidate {
	if ev.Kind != "commit" || ev.Commit.Operation != "create" ||
		ev.Commit.Collection != "app.bsky.feed.post" {
		return nil
	}
	r := ev.Commit.Record

	// English only. Not a judgement about other languages — this instance
	// cannot read them, so surfacing one would be posting something nobody
	// here has understood.
	//
	// Two checks, because the tag is self-declared. A Japanese post tagged
	// "en" reached micro.mu's feed past both of the filters that read the tag
	// and nothing else: worthParsing scans the bytes for "en inside langs, and
	// this asked the parsed field the same question. Whoever posted it was not
	// lying, they were using a client that defaults to en — which is why
	// trusting the tag alone was never going to hold.
	if !hasLang(r.Langs, "en") {
		return nil
	}
	// Replies are half of everything and make no sense alone: a reply is an
	// answer to a post the reader has not seen.
	if r.Reply != nil {
		return nil
	}
	text := strings.TrimSpace(r.Text)
	// Everything that reads the post for meaning reads it without its links.
	prose := stripLinks(text)
	if len(prose) < minText {
		return nil
	}
	// And the text has to look like what the tag claims. See latinScript.
	if !latinScript(prose) {
		return nil
	}

	link, media, tags := contents(r)
	// Something to look at. A post with neither a link nor media is a thought,
	// and a stranger's thought out of context is the noise this is avoiding.
	if link == "" && !media {
		return nil
	}
	if tags > maxTags {
		return nil
	}

	cat := categoryOf(prose)
	if cat == "" {
		return nil
	}
	if selling(prose) || mirrored(text) {
		return nil
	}
	return &candidate{
		DID: ev.DID, Text: text, Link: link, Category: cat, Media: media,
		Score: score(prose, link, media, tags), At: time.Now(),
	}
}

// stripLinks removes addresses before anything reads the text for meaning.
//
// Bluesky puts a link into the post's text as its own domain and path, so the
// text carries strings like "www.liverpoolmuseums.org.uk/artifact/bes...". A
// post about a 2,500-year-old Egyptian jar came back from the live run filed
// under UK, on the "uk" in that hostname. The next run filed a dementia course
// under UK too, on "dementia@worc.ac.uk" — an email address, and handles like
// "@totalwar.bsky.social" read the same way.
//
// A URL, an email address and a handle are all addresses rather than sentences:
// none should count towards either the category or the length.
func stripLinks(text string) string {
	fields := strings.Fields(text)
	out := fields[:0]
	for _, f := range fields {
		l := strings.ToLower(f)
		if strings.Contains(l, "://") || strings.HasPrefix(l, "www.") ||
			strings.Contains(l, "@") ||
			(strings.Contains(l, "/") && strings.Contains(l, ".")) {
			continue
		}
		out = append(out, f)
	}
	return strings.Join(out, " ")
}

// selling refuses the post that is an advert.
//
// A job advert is long, in English, links out, and is squarely in a category —
// it passes every test above by construction, and the live run surfaced one
// under UK ("an opportunity for an Engine QA Analyst… in Sussex") and a sales
// pitch under Tech ("Watch now :)"). Nothing here is a judgement about quality:
// a post asking the reader to apply, buy or subscribe is addressed to a market
// rather than to a reader, and this instance is not somebody's distribution.
//
// Phrases, not words, because the words are all innocent on their own — a story
// about a company hiring is worth reading, an invitation to apply is not.
func selling(prose string) bool {
	t := strings.ToLower(prose)
	for _, p := range adverts {
		if strings.Contains(t, p) {
			return true
		}
	}
	return false
}

var adverts = []string{
	"we're hiring", "we are hiring", "now hiring", "join our team", "apply now",
	"an opportunity for", "job alert", "open role", "we have a vacancy",
	"link in bio", "use code", "discount code", "promo code", "giveaway",
	"sign up now", "subscribe to my", "book a demo", "limited time offer",
	"watch now", "out now on", "pre-order", "buy now",
	"book now", "register now", "join our", "places are limited", "tickets available",
}

// mirrored refuses a post that is a machine reposting somebody else's writing.
//
// The same mirror came back twice in five live runs, ending every post
// "[Original post on igeek.gamer-geek-news.com]" around an article body cut off
// mid-sentence with "[…]". A repost bot clears every mechanical filter here by
// construction — it is long, English, in a category and links out — and the two
// marks it leaves are the ones no person writing a post would leave: an
// attribution in brackets, and a truncation mark where the article ran out.
func mirrored(text string) bool {
	t := strings.ToLower(text)
	return strings.Contains(t, "[…]") || strings.Contains(t, "[...]") ||
		strings.Contains(t, "[original post")
}

func hasLang(langs []string, want string) bool {
	for _, l := range langs {
		if strings.EqualFold(l, want) || strings.HasPrefix(strings.ToLower(l), want+"-") {
			return true
		}
	}
	return false
}

// latinScript reports whether the prose is written in the alphabet English is
// written in.
//
// The second half of the language check, and the one that does not take the
// author's word for it. It answers a narrow question — which script — and that
// is deliberate: script is a fact about the bytes, decidable here in
// microseconds against three million posts a day, where the language is not.
//
// So it catches a post in Japanese, Chinese, Korean, Arabic, Hebrew, Russian,
// Greek, Thai or Hindi tagged as English, and it does not catch one in
// Portuguese or Spanish tagged the same way. That gap is real and left open:
// closing it means identifying a language rather than a script, which is a
// model call per post on a firehose sample, and this runs before the shortlist
// the model actually reads.
//
// A share rather than a count, because an English post may quote a name, a
// place or a word in another script and still be an English post. Only letters
// are counted: emoji, punctuation and digits belong to no script and say
// nothing about which language this is.
func latinScript(prose string) bool {
	var letters, latin int
	for _, r := range prose {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		if unicode.Is(unicode.Latin, r) {
			latin++
		}
	}
	// Nothing to judge. A post of only emoji and links is refused elsewhere for
	// having no prose; refusing it here as well would be this function
	// answering a question it was not asked.
	if letters == 0 {
		return true
	}
	return float64(latin)/float64(letters) >= latinShare
}

// contents pulls the first external link, whether there is media, and how many
// hashtags the post carries.
func contents(r record) (link string, media bool, tags int) {
	for _, f := range r.Facets {
		for _, feat := range f.Features {
			switch {
			case strings.Contains(feat.Type, "#link") && link == "":
				link = feat.URI
			case strings.Contains(feat.Type, "#tag"):
				tags++
			}
		}
	}
	if r.Embed != nil {
		if r.Embed.External != nil && link == "" {
			link = r.Embed.External.URI
		}
		if len(r.Embed.Images) > 0 || r.Embed.Video != nil {
			media = true
		}
	}
	return link, media, tags
}

// score ranks what survived, so a review can take the best rather than the
// first. Deliberately arithmetic and not a model call: this runs on every post
// that passes, and a judgement worth paying for is one made over a shortlist.
func score(text, link string, media bool, tags int) int {
	// Length counts, but not much and not for long. Dividing a 400-character cap
	// by 20 gave length twenty of the available points, so every post over the
	// cap scored the same and the ranking inside a category was whatever arrived
	// first — three posts came back from the first live run scoring 26 apiece.
	n := len(text)
	if n > 300 {
		n = 300
	}
	s := n / 50 // longer is usually more considered, to a point
	if link != "" {
		s += 8 // somebody is pointing at something outside themselves
	}
	if media {
		s += 3
	}
	s -= tags * 2
	if strings.Count(text, "http") > 2 {
		s -= 5 // a list of links is an aggregator, not a person
	}
	if inward[hostOf(link)] {
		// A link back into another feed is not pointing outside itself, and a
		// shortener is a link that will not say where it goes.
		s -= 6
	}
	return s
}

// inward are the hosts that are not somewhere to go: other feeds, and
// shorteners, which hide the answer to the only question worth asking about a
// stranger's link.
var inward = map[string]bool{
	"bsky.app": true, "x.com": true, "twitter.com": true, "instagram.com": true,
	"facebook.com": true, "threads.net": true, "tiktok.com": true,
	"bit.ly": true, "t.co": true, "tinyurl.com": true, "buff.ly": true, "ow.ly": true,
}

// categoryOf matches a post to one of the categories this instance already
// organises everything else by. Keyword matching, deliberately: it is
// deterministic, it costs nothing on a firehose, and a post that needs a model
// to work out which category it is in probably belongs in none of them.
func categoryOf(text string) string {
	lower := strings.ToLower(text)
	// Offsets into the lowercased text only address the original when lowering
	// did not change its length, which for a handful of non-ASCII letters it
	// does. When it has, take the match and skip the name test rather than read
	// the wrong bytes.
	aligned := len(lower) == len(text)
	for _, c := range categories {
		for _, kw := range c.words {
			for _, i := range occurrences(lower, kw) {
				if !aligned || !partOfAName(text, i, len(kw)) {
					return c.name
				}
			}
		}
	}
	return ""
}

// containsWord matches a whole word, so "ai" does not match "said" and "uk"
// does not match "duke".
func containsWord(text, word string) bool { return len(occurrences(text, word)) > 0 }

// occurrences lists every whole-word position of word in an already-lowercased
// text.
func occurrences(text, word string) []int {
	var out []int
	for i := 0; ; {
		j := strings.Index(text[i:], word)
		if j < 0 {
			return out
		}
		j += i
		end := j + len(word)
		if (j == 0 || !isWordChar(text[j-1])) && (end >= len(text) || !isWordChar(text[end])) {
			out = append(out, j)
		}
		i = j + len(word)
	}
}

// partOfAName reports whether an occurrence is somebody's name rather than the
// subject of the post.
//
// The live run surfaced a King's College research award under Islam, because
// one of the two people congratulated was Dr Jasmin Islam. A keyword cannot
// tell a topic from a surname, but the shape around it can: a capitalised word
// directly after another capitalised word is a name, and a lowercase one never
// is. An acronym is exempt — NHS in "Barts Health NHS Trust" is capitalised
// after a capital and is still the health service.
//
// This loses the occasional post written in Title Case. That is the right side
// to err on: there are three million posts a day and three slots a review, so
// recall is the abundant thing here and precision is the scarce one.
func partOfAName(orig string, i, n int) bool {
	if i+n > len(orig) {
		return false
	}
	word := orig[i : i+n]
	if !upper(word[0]) {
		return false
	}
	if allUpper(word) {
		return false // an acronym, not a name
	}
	// The word immediately before it.
	j := i - 1
	for j >= 0 && orig[j] == ' ' {
		j--
	}
	if j == i-1 {
		return false // nothing between them, so not two words
	}
	end := j + 1
	for j >= 0 && isWordChar(orig[j]) {
		j--
	}
	prev := orig[j+1 : end]
	return prev != "" && upper(prev[0])
}

func upper(b byte) bool { return b >= 'A' && b <= 'Z' }

func allUpper(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 'a' && s[i] <= 'z' {
			return false
		}
	}
	return true
}

func isWordChar(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

// categories are the ones this instance already sorts news by, so social does
// not invent a second taxonomy for the same world.
//
// A keyword has to mean the category almost every time it appears, because
// nothing downstream re-checks it. The first live run surfaced a physiotherapy
// conference programme under Politics: it said "policy" and it said "congress",
// and a European medical congress about professional policy is neither. Both
// words are gone. The test for a keyword is not "does this word turn up in
// political writing" — it is "when this word turns up, is it political".
var categories = []struct {
	name  string
	words []string
}{
	{"Crypto", []string{"bitcoin", "ethereum", "crypto", "stablecoin", "defi", "solana", "onchain", "btc", "eth"}},
	{"Dev", []string{"golang", "rust", "kubernetes", "postgres", "compiler", "open source", "typescript", "sdk", "devtools"}},
	{"Tech", []string{"ai", "llm", "openai", "anthropic", "nvidia", "chip", "semiconductor", "startup", "software", "robotics"}},
	{"Finance", []string{"inflation", "interest rates", "nasdaq", "earnings", "recession", "central bank", "bond market"}},
	{"Politics", []string{"election", "parliament", "senate", "white house", "prime minister", "legislation", "sanctions", "referendum", "geopolitics"}},
	{"UK", []string{"uk", "britain", "british", "london", "nhs", "westminster", "scotland", "wales"}},
	{"World", []string{"gaza", "ukraine", "china", "india", "africa", "united nations", "climate change"}},
	{"Islam", []string{"islam", "muslim", "quran", "ramadan", "mosque", "hajj", "palestine"}},
}

// ── Keeping and choosing ────────────────────────────────────────

// keep buffers a candidate, dropping the weakest when full.
func keep(c *candidate) {
	mu.Lock()
	defer mu.Unlock()
	if surfaced[c.Link] && c.Link != "" {
		return
	}
	candidates = append(candidates, c)
	if len(candidates) > maxCandidates {
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
		candidates = candidates[:maxCandidates]
	}
}

// review decides what the window was worth and publishes it.
//
// Two stages, and the split is the point. Arithmetic narrows a batch to a
// shortlist — that has to be cheap, because it runs against everything. Then a
// model reads the shortlist and picks, which is the judgement the arithmetic
// cannot make: a scorer can tell a long post with a link from a short one
// without, and cannot tell an interesting story from a dull one.
func review() {
	mu.Lock()
	batch := candidates
	candidates = nil
	mu.Unlock()

	if len(batch) == 0 {
		return
	}
	short := shortlist(batch)
	chosen := judge(short)
	if len(chosen) == 0 {
		// No model, or it had nothing to say. The arithmetic order is what this
		// did before there was a judge, and it is a reasonable answer.
		chosen = short
	}

	posted := 0
	for _, c := range chosen {
		if posted >= surfacePerReview {
			break
		}
		if !claim(c) {
			continue
		}
		posted++
		Surface(c)
	}
	if posted > 0 {
		app.Log("social", "atproto: surfaced %d of %d candidates", posted, len(batch))
	}
}

// shortlist is the arithmetic pass: the best-scoring candidates, one per
// category and one per author, ordered.
//
// One per category, because a busy hour in one category is exactly what a busy
// hour looks like, and without the cap it takes the whole batch. One per
// author, because the accounts that post most are the ones posting
// automatically — a mirror that reposts a feed clears every mechanical filter
// by construction, and the thing it cannot do is be several people.
func shortlist(batch []*candidate) []*candidate {
	sort.SliceStable(batch, func(i, j int) bool { return batch[i].Score > batch[j].Score })
	seenCat, seenWho := map[string]bool{}, map[string]bool{}
	var out []*candidate
	for _, c := range batch {
		if seenCat[c.Category] || (c.DID != "" && seenWho[c.DID]) {
			continue
		}
		seenCat[c.Category], seenWho[c.DID] = true, true
		out = append(out, c)
	}
	return out
}

// claim records that a link has been surfaced, and reports whether this call is
// the one that got there first. A story doing the rounds is one story.
func claim(c *candidate) bool {
	if c.Link == "" {
		return true
	}
	mu.Lock()
	defer mu.Unlock()
	if surfaced[c.Link] {
		return false
	}
	surfaced[c.Link] = true
	if len(surfaced) > 5000 {
		surfaced = map[string]bool{c.Link: true}
	}
	return true
}

// display is the text as a reader should see it: the post, and where it points.
func (c *candidate) display() string {
	text := c.Text
	if len(text) > 400 {
		text = text[:397] + "..."
	}
	return text
}

// host is the link's domain, which is the most useful single thing to know
// about a stranger's link before clicking it.
func (c *candidate) host() string { return hostOf(c.Link) }

func hostOf(link string) string {
	if link == "" {
		return ""
	}
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimPrefix(u.Host, "www."))
}

// Surface is what the agent decided, handed to the service to store. Assigned
// at boot so this package can be tested without standing up social.
var Surface = func(c *candidate) {}
