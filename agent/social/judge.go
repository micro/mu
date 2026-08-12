package social

// Choosing, once the machine has finished refusing.
//
// Everything in jetstream.go is mechanical: facts about a post, decided by
// arithmetic, run against three million posts a day. That gets a window's worth
// of firehose down to a shortlist of a dozen, and then it stops being able to
// help. A scorer can tell a long post with a link from a short one without. It
// cannot tell an SEC filing on an ETF liquidation from a press release about a
// dementia course, and both of those came back from live runs looking identical
// to the arithmetic.
//
// So the last step is a judgement, and a judgement is what a model is for. It
// costs one call every fifteen minutes over a list of a dozen short posts,
// which is the cheapest thing in this repo that a model does — and it is the
// step the arithmetic was only ever approximating.
//
// The instance is not required to have a model. Without one the shortlist is
// published in score order, which is what this did before there was a judge.

import (
	"fmt"
	"strconv"
	"strings"

	"mu/internal/ai"
	"mu/internal/app"
)

// shortlistMax bounds what the model is asked to read. A dozen posts is a
// choice; fifty is a reading task, and the shortlist is already ordered, so the
// tail is the part least likely to be picked anyway.
const shortlistMax = 12

// judgeChars is how much of a post the model sees. Enough to tell what it is
// about, short enough that a dozen of them is a small prompt.
const judgeChars = 300

// judge asks the model which of the shortlist are worth publishing, and returns
// them in its order. An empty result means no model, no answer, or nothing it
// thought was worth it — the caller falls back to the arithmetic order.
func judge(short []*candidate) []*candidate {
	if len(short) == 0 || !ai.Configured() {
		return nil
	}
	if len(short) > shortlistMax {
		short = short[:shortlistMax]
	}
	// One is not a choice.
	if len(short) < 2 {
		return short
	}

	answer, err := ai.Ask(&ai.Prompt{
		System:    judgeSystem,
		Question:  numbered(short),
		Priority:  ai.PriorityLow,
		MaxTokens: 200,
		Caller:    "social-judge",
	})
	if err != nil {
		app.Log("social", "atproto: judge unavailable (%v), publishing by score", err)
		return nil
	}
	picked := picks(answer, len(short))
	if len(picked) == 0 {
		// A model that picks nothing has said something: this batch was thin.
		// Publishing the top of it anyway would be overruling the only part of
		// this pipeline that can actually read.
		app.Log("social", "atproto: judge found nothing worth surfacing in %d", len(short))
		return nil
	}
	out := make([]*candidate, 0, len(picked))
	for _, i := range picked {
		out = append(out, short[i])
	}
	return out
}

const judgeSystem = `You are choosing which posts from the open social network are worth putting on a public timeline that people read for news and ideas.

You will be given a numbered list of posts. Each has a category, the domain it links to, and its text.

Pick at most 3 that a thoughtful reader would be glad to have seen. Prefer:
- something that happened, or a substantive argument about something that happened
- a primary source, or a publication with a name
- a post that would still make sense to somebody who does not follow the author

Reject, however well it scores:
- advertising, recruitment, self-promotion, or anything asking the reader to buy, apply, book or subscribe
- a press release, a conference programme, or an announcement of an announcement
- engagement bait, a thread opener, or a post whose whole content is a reaction
- anything you cannot tell the subject of from the text given

Pick fewer than 3 if fewer are worth it. Pick none if none are — that is a real answer and an empty line is how you give it.

Reply with only the numbers you have picked, best first, separated by commas. No other words, no explanation.`

// numbered renders the shortlist for the model.
func numbered(short []*candidate) string {
	var b strings.Builder
	for i, c := range short {
		text := strings.Join(strings.Fields(c.Text), " ")
		if len(text) > judgeChars {
			text = text[:judgeChars] + "..."
		}
		where := c.host()
		if where == "" {
			where = "no link"
		}
		fmt.Fprintf(&b, "%d. [%s] (%s) %s\n\n", i+1, c.Category, where, text)
	}
	return strings.TrimSpace(b.String())
}

// picks reads the model's answer as a list of shortlist indexes.
//
// Deliberately forgiving about the shape of the reply and strict about the
// content: anything that is not a number in range is dropped rather than
// guessed at. A model that answers "1, 3" and one that answers "I'd pick 1 and
// 3." both mean the same thing, and neither should be able to publish post 7.
func picks(answer string, n int) []int {
	var out []int
	seen := map[int]bool{}
	for _, field := range strings.FieldsFunc(answer, func(r rune) bool {
		return r < '0' || r > '9'
	}) {
		v, err := strconv.Atoi(field)
		if err != nil || v < 1 || v > n || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v-1)
		if len(out) == surfacePerReview {
			break
		}
	}
	return out
}
