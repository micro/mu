package social

import (
	"encoding/json"
	"strings"
	"testing"
)

// post builds a Jetstream event the way the network actually sends one — the
// shape was taken from a live connection rather than from the documentation.
func post(text string, opts ...func(map[string]any)) event {
	rec := map[string]any{
		"$type": "app.bsky.feed.post", "text": text, "langs": []string{"en"},
	}
	for _, o := range opts {
		o(rec)
	}
	raw, _ := json.Marshal(map[string]any{
		"did": "did:plc:someone", "kind": "commit", "time_us": 1786527580809679,
		"commit": map[string]any{
			"operation": "create", "collection": "app.bsky.feed.post", "record": rec,
		},
	})
	var ev event
	json.Unmarshal(raw, &ev) //nolint:errcheck
	return ev
}

func withLink(uri string) func(map[string]any) {
	return func(r map[string]any) {
		r["facets"] = []any{map[string]any{"features": []any{
			map[string]any{"$type": "app.bsky.richtext.facet#link", "uri": uri},
		}}}
	}
}

func withTags(n int) func(map[string]any) {
	return func(r map[string]any) {
		var feats []any
		for i := 0; i < n; i++ {
			feats = append(feats, map[string]any{"$type": "app.bsky.richtext.facet#tag", "tag": "x"})
		}
		r["facets"] = []any{map[string]any{"features": feats}}
	}
}

func withImages(r map[string]any) {
	r["embed"] = map[string]any{"$type": "app.bsky.embed.images", "images": []any{map[string]any{}}}
}

func withLang(l string) func(map[string]any) {
	return func(r map[string]any) { r["langs"] = []string{l} }
}

func asReply(r map[string]any) {
	r["reply"] = map[string]any{"root": map[string]any{"uri": "at://x"}}
}

const techPost = "Really useful writeup on how the new golang compiler optimisations change escape analysis in practice, with benchmarks"

// TestTheNoiseIsRefusedCheaply — three million posts a day is not a feed, it is
// weather. Everything here is a fact about the post rather than a judgement.
func TestTheNoiseIsRefusedCheaply(t *testing.T) {
	cases := []struct {
		name string
		ev   event
	}{
		{"a reply, which makes no sense without the post it answers",
			post(techPost, withLink("https://example.com/a"), asReply)},
		{"a language nobody here can read",
			post(techPost, withLink("https://example.com/a"), withLang("ja"))},
		{"too short to be worth reading somewhere else",
			post("golang is good", withLink("https://example.com/a"))},
		{"nothing to look at — no link, no media",
			post(techPost)},
		{"addressed to a search index rather than a person",
			post(techPost, withTags(8))},
		{"about nothing this instance covers",
			post("Long ramble about my breakfast and the weather and the dog and the garden today", withLink("https://example.com/a"))},
	}
	for _, c := range cases {
		if got := consider(c.ev); got != nil {
			t.Errorf("kept %s: %+v", c.name, got)
		}
	}
}

func TestSomethingWorthReadingSurvives(t *testing.T) {
	got := consider(post(techPost, withLink("https://blog.example.com/escape-analysis")))
	if got == nil {
		t.Fatal("a long English post with a link about a category was refused")
	}
	if got.Category != "Dev" {
		t.Errorf("category = %q, want Dev", got.Category)
	}
	if got.Link != "https://blog.example.com/escape-analysis" {
		t.Errorf("link = %q", got.Link)
	}
	if got.host() != "blog.example.com" {
		t.Errorf("host = %q", got.host())
	}

	// Media alone is enough to be worth looking at.
	if consider(post(techPost, withImages)) == nil {
		t.Error("a post with images and no link was refused")
	}
}

// TestOnlyCreatesAreConsidered — an edit or a delete is not news.
func TestOnlyCreatesAreConsidered(t *testing.T) {
	ev := post(techPost, withLink("https://example.com/a"))
	ev.Commit.Operation = "delete"
	if consider(ev) != nil {
		t.Error("a delete was treated as a post")
	}
}

// TestWholeWordsOnly — "ai" must not match "said", "uk" must not match "duke".
func TestWholeWordsOnly(t *testing.T) {
	if categoryOf("he said the thing about his duke and his aim") != "" {
		t.Error("a substring matched a category keyword")
	}
	if categoryOf("the uk parliament voted") == "" {
		t.Error("a real keyword did not match")
	}
}

// TestAKeywordHasToMeanTheCategory. The first live run surfaced a physiotherapy
// conference programme under Politics, because it contained "policy" and
// "congress". The bar for a keyword is not that it appears in political writing
// — it is that when it appears, the post is political.
func TestAKeywordHasToMeanTheCategory(t *testing.T) {
	for _, text := range []string{
		`Focused Symposium | 6th Europe Region Physiotherapy Congress. ` +
			`"Professional Autonomy in Physiotherapy: Empowerment Through Policy, Practice and Education". ` +
			`Discover all the sessions.`,
		"Our privacy policy has been updated, please review the changes to how we handle your data",
		"The annual congress of the European Society of Cardiology opens tomorrow in Milan",
	} {
		if got := categoryOf(strings.ToLower(text)); got == "Politics" {
			t.Errorf("filed under Politics: %s", text)
		}
	}
	// The category still has to work.
	if categoryOf("the senate voted on the sanctions legislation last night") != "Politics" {
		t.Error("real political writing no longer matches Politics")
	}
}

// TestAnAdvertIsNotAStory — a job advert passes every mechanical test above by
// construction: long, English, links out, squarely in a category.
func TestAnAdvertIsNotAStory(t *testing.T) {
	for _, text := range []string{
		"We have an opportunity for an Engine QA Analyst to join our QA team in Sussex, UK, " +
			"helping us craft the franchise. You will work across manual and automated testing.",
		"The 13 objections we hear most from B2B software companies on sales calls. " +
			"And my responses to each. Mix of rant and educational outbound stuff. Watch now :)",
		"Our new golang course is live — use code LAUNCH for 40% off, this week only, " +
			"covering compilers, concurrency and everything in between",
	} {
		if consider(post(text, withLink("https://example.com/a"))) != nil {
			t.Errorf("kept an advert: %s", text)
		}
	}
	// A mirror reposting somebody else's article is not a post.
	if consider(post("🤖 AI was supposed to destroy jobs. Where's the carnage? The AI jobs "+
		"apocalypse never showed up. Still, jobs are changing and economists expect en […] "+
		"[Original post on igeek.gamer-geek-news.com]", withLink("https://example.com/a"))) != nil {
		t.Error("kept a repost bot")
	}

	// A story about hiring is still a story.
	if consider(post("Nvidia is hiring aggressively for its compiler team, according to filings "+
		"published this week, which says something about where the chip roadmap is going",
		withLink("https://example.com/a"))) == nil {
		t.Error("refused a story about a company hiring")
	}
}

// TestANameIsNotASubject. The live run surfaced a King's College research award
// under Islam, because one of the people congratulated was Dr Jasmin Islam.
func TestANameIsNotASubject(t *testing.T) {
	name := "Congratulations to Dr Daniel Hadfield and Dr Jasmin Islam, members of the " +
		"Centre for Critical Illness Research, who have received Senior Research Awards."
	if got := categoryOf(name); got != "" {
		t.Errorf("a surname decided the category: %q", got)
	}
	// The subject still matches.
	for _, subject := range []string{
		"A thoughtful essay on how Islam understands debt and interest, worth reading in full",
		"islam and the ethics of lending, a long read",
	} {
		if categoryOf(subject) != "Islam" {
			t.Errorf("the subject no longer matches: %q", subject)
		}
	}
	// An acronym is capitalised after a capital and is still not a name.
	if categoryOf("Barts Health NHS Trust shipped ventilators and held solidarity meetings") != "UK" {
		t.Error("an acronym was mistaken for a surname")
	}
}

// TestALinkIsNotASentence. Bluesky renders a link in the post's text as its own
// hostname and path, so the text of a post about an Egyptian jar in a Liverpool
// museum contains "liverpoolmuseums.org.uk/..." — and the live run filed it
// under UK on the strength of that. No filter here should read a URL.
func TestALinkIsNotASentence(t *testing.T) {
	const jar = "Ancient Egyptian Bes-shaped ceramic jar, dated 600 BCE, from Fayum. " +
		"World Museum. www.liverpoolmuseums.org.uk/artifact/bes-jar"
	if got := categoryOf(strings.ToLower(stripLinks(jar))); got != "" {
		t.Errorf("a hostname decided the category: %q", got)
	}
	// An email address and a handle are addresses too. A dementia course came
	// back filed under UK on "dementia@worc.ac.uk".
	const course = "Come and learn how to better support people living with advanced dementia " +
		"and at the end of life on this online course. Contact us dementia@worc.ac.uk"
	if got := categoryOf(stripLinks(course)); got != "" {
		t.Errorf("an email address decided the category: %q", got)
	}

	// A post that is a link and little else is a short post.
	short := "look at this www.example.co.uk/some/very/long/path/that/pads/the/length/out/nicely"
	if len(stripLinks(short)) >= minText {
		t.Error("a URL padded a post past the length floor")
	}
	// And real prose still survives being stripped.
	if categoryOf(strings.ToLower(stripLinks(techPost+" https://example.com/a"))) != "Dev" {
		t.Error("stripping links took the sentence with it")
	}
}

// TestALinkHasToGoSomewhere — a link back into another feed, or into a
// shortener, is not somebody pointing outside themselves.
func TestALinkHasToGoSomewhere(t *testing.T) {
	const text = techPost
	out := score(text, "https://blog.example.com/escape-analysis", false, 0)
	back := score(text, "https://bsky.app/profile/someone/post/abc", false, 0)
	short := score(text, "https://bit.ly/3xyz", false, 0)

	if back >= out {
		t.Errorf("a link back into a feed scored %d against %d for a link out", back, out)
	}
	if short >= out {
		t.Errorf("a shortener scored %d against %d for a link that says where it goes", short, out)
	}
}

// TestScoreStillSeparatesLongPosts — length used to be worth twenty of the
// available points against a cap of 400 characters, so everything over the cap
// tied and the ranking inside a category was arrival order. Three posts came
// back from the first live run scoring 26 apiece.
func TestScoreStillSeparatesLongPosts(t *testing.T) {
	long := strings.Repeat("a considered paragraph about golang. ", 40) // well past any cap
	withLink := score(long, "https://example.com/a", false, 0)
	tagged := score(long, "https://example.com/a", false, 3)
	if withLink <= tagged {
		t.Errorf("hashtags did not separate two otherwise identical long posts: %d vs %d", withLink, tagged)
	}
	if media := score(long, "https://example.com/a", true, 0); media <= withLink {
		t.Errorf("media did not separate two otherwise identical long posts: %d vs %d", media, withLink)
	}
}

// TestReviewTakesTheBestNotTheFirst, and never floods one category.
func TestReviewTakesTheBestNotTheFirst(t *testing.T) {
	var got []*candidate
	Surface = func(c *candidate) { got = append(got, c) }
	defer func() { Surface = func(*candidate) {} }()

	candidates = nil
	surfaced = map[string]bool{}
	for i, c := range []*candidate{
		{Category: "Tech", Link: "https://a", Score: 1},
		{Category: "Tech", Link: "https://b", Score: 99}, // best, and same category
		{Category: "Dev", Link: "https://c", Score: 50},
		{Category: "UK", Link: "https://d", Score: 40},
		{Category: "World", Link: "https://e", Score: 30},
	} {
		_ = i
		candidates = append(candidates, c)
	}
	review()

	if len(got) != surfacePerReview {
		t.Fatalf("surfaced %d, want %d", len(got), surfacePerReview)
	}
	if got[0].Link != "https://b" {
		t.Errorf("first surfaced was %s, want the highest scoring", got[0].Link)
	}
	seen := map[string]bool{}
	for _, c := range got {
		if seen[c.Category] {
			t.Errorf("surfaced %s twice — one busy category takes the whole batch", c.Category)
		}
		seen[c.Category] = true
	}
}

// TestOneVoiceCannotTakeTheBatch — the accounts that post most are the ones
// posting automatically, and a repost bot clears every mechanical filter here
// by construction. What it cannot do is be several people.
func TestOneVoiceCannotTakeTheBatch(t *testing.T) {
	var got []*candidate
	Surface = func(c *candidate) { got = append(got, c) }
	defer func() { Surface = func(*candidate) {} }()

	candidates, surfaced = nil, map[string]bool{}
	for _, c := range []*candidate{
		{DID: "did:plc:bot", Category: "Tech", Link: "https://a", Score: 90},
		{DID: "did:plc:bot", Category: "Dev", Link: "https://b", Score: 80},
		{DID: "did:plc:bot", Category: "UK", Link: "https://c", Score: 70},
		{DID: "did:plc:person", Category: "World", Link: "https://d", Score: 10},
	} {
		candidates = append(candidates, c)
	}
	review()

	if len(got) != 2 {
		t.Fatalf("surfaced %d, want one from the bot and one from the person", len(got))
	}
	if got[1].DID != "did:plc:person" {
		t.Errorf("the second slot went to %s", got[1].DID)
	}
}

// TestTheSameLinkIsNotSurfacedTwice — a story doing the rounds is one story.
func TestTheSameLinkIsNotSurfacedTwice(t *testing.T) {
	var got []*candidate
	Surface = func(c *candidate) { got = append(got, c) }
	defer func() { Surface = func(*candidate) {} }()

	candidates, surfaced = nil, map[string]bool{}
	candidates = append(candidates, &candidate{Category: "Tech", Link: "https://same", Score: 10})
	review()
	candidates = append(candidates, &candidate{Category: "Tech", Link: "https://same", Score: 90})
	review()

	if len(got) != 1 {
		t.Errorf("the same link was surfaced %d times", len(got))
	}
}

func TestDisplayIsBounded(t *testing.T) {
	c := &candidate{Text: strings.Repeat("x", 900)}
	if len(c.display()) > 410 {
		t.Errorf("a post was rendered at %d characters", len(c.display()))
	}
}

func TestOffUnlessAnOperatorTurnsItOn(t *testing.T) {
	t.Setenv("SOCIAL_ATPROTO", "")
	if Enabled() {
		t.Error("watching the network is on by default — that is a decision about " +
			"what the operator publishes, and it is theirs")
	}
	t.Setenv("SOCIAL_ATPROTO", "true")
	if !Enabled() {
		t.Error("SOCIAL_ATPROTO=true did not turn it on")
	}
}
