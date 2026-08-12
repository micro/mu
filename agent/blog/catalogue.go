package blog

// What the instance has to say today, asked of the catalogue rather than of
// five packages somebody chose once.
//
// gatherCategoryContext used to call news.GetFeed, markets.GetAllPriceData,
// video.GetLatestVideos and prayer.GetReminderData — four services, named in
// code. Flights shipped and the digest could not mention it, and a self-hoster
// who adds their own service would never see it appear, because nothing here
// was asking what exists. It was asking four packages how they were.
//
// Now it asks the registry. Every registered service is offered the chance to
// contribute; whatever answers goes in the prompt under its own heading. Add a
// service and the next digest can talk about it, with nobody editing this file.
//
// Two things make that safe to do blindly.
//
// A service is only asked if the call is free, unscoped and not destructive —
// which is derivable from its own Spec, so the decision is the service's rather
// than a list kept here. Nothing account-scoped is touched: the digest is
// written by the instance about the world, and one person's mail is neither.
//
// And the answers are prose. That is exactly right here and worth saying,
// because it looked like a limitation: the digest is assembling a *prompt*, not
// computing anything, so text is the shape it wants. What a program would need
// — the structured half — is a different problem, and it is not this one.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"mu/internal/service"
)

// contributor is one service's offer of material, in the order it should be read.
type contributor struct {
	service  string
	endpoint string
	args     map[string]any
	heading  string
	rank     int
}

// gatherTimeout bounds one service's contribution. A digest is written once a
// day and can afford to wait; it cannot afford to hang forever on a provider
// having a bad morning.
const gatherTimeout = 30 * time.Second

// maxPerContribution caps what any one service can put into the prompt. Without
// it a service with a long list crowds out every other, and the model reads
// whichever one happened to be verbose.
const maxPerContribution = 2000

// contributors asks the catalogue who has something to say.
//
// Read-shaped endpoints only: List, and the handful of names a service uses for
// the same idea. Anything that creates, sends, deletes or costs money is not
// material for an opinion piece, and the Spec already says which is which.
func contributors(category string) []contributor {
	var out []contributor
	for _, spec := range service.Specs() {
		if spec.Scoped {
			continue // somebody's own data, not the world's
		}
		for name, ep := range spec.Endpoints {
			if !readShaped(name) || ep.Destructive || ep.Cost != "" {
				continue
			}
			c := contributor{
				service:  spec.Name,
				endpoint: name,
				args:     map[string]any{},
				heading:  headingFor(spec, name),
				rank:     rankOf(spec.Name),
			}
			// News is the spine of an opinion piece, so it is asked about this
			// category specifically. Every other service is asked for whatever
			// it has; a market price is not about "tech".
			if spec.Name == "news" && category != "" {
				c.args["topic"] = category
			}
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].rank != out[j].rank {
			return out[i].rank < out[j].rank
		}
		return out[i].service < out[j].service
	})
	return out
}

// readShaped reports whether a method name is one a service uses to answer
// "what have you got". List is the convention; the others are services that
// named the same idea differently before the convention existed.
func readShaped(method string) bool {
	switch method {
	case "List", "Times", "Reflection", "Forecast":
		return true
	}
	return false
}

// rankOf puts the material a reader expects first. Anything not named here
// still contributes — it just does not get to lead, which is the difference
// between an ordering and an allowlist.
func rankOf(svc string) int {
	switch svc {
	case "news":
		return 0
	case "markets":
		return 1
	case "prayer":
		return 2
	}
	return 3
}

// headingFor names a section the way a reader would.
func headingFor(spec service.Spec, method string) string {
	label := spec.Label
	if label == "" {
		label = strings.ToUpper(spec.Name[:1]) + spec.Name[1:]
	}
	if method == "List" {
		return label
	}
	return label + " — " + method
}

// gatherFromCatalogue builds the prompt material for a category.
func gatherFromCatalogue(category string) string {
	var b strings.Builder
	if category != "" {
		fmt.Fprintf(&b, "# Material for a piece on %s\n\n", category)
	}

	got := 0
	for _, c := range contributors(category) {
		text := ask(c)
		if text == "" {
			continue
		}
		if len(text) > maxPerContribution {
			text = text[:maxPerContribution] + "\n[truncated]"
		}
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", c.heading, text)
		got++
	}
	if got == 0 {
		return ""
	}
	return b.String()
}

// ask calls one contributor and returns whatever prose it gave back.
//
// A service that errors, times out or has nothing today contributes nothing and
// says nothing about it. One quiet provider must not stop the digest: the whole
// point of asking the catalogue is that the answer is whoever happens to be
// able to answer.
func ask(c contributor) string {
	ctx, cancel := context.WithTimeout(context.Background(), gatherTimeout)
	defer cancel()

	rsp, err := service.CallDynamic(ctx, c.service, c.endpoint, c.args)
	if err != nil {
		return ""
	}
	return prose(rsp)
}

// prose pulls the readable part out of a response.
//
// Every service answers with a text field today, and that is the convention
// rather than a guarantee, so this looks for the usual names and then for any
// string long enough to be worth reading. A service that answers in some shape
// nobody anticipated contributes nothing rather than contributing a Go map
// printed into a prompt.
func prose(rsp map[string]any) string {
	for _, key := range []string{"text", "Text", "reminder", "Reminder", "content", "Content"} {
		if s, ok := rsp[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	best := ""
	for _, v := range rsp {
		if s, ok := v.(string); ok && len(s) > len(best) {
			best = s
		}
	}
	if len(strings.TrimSpace(best)) < 40 {
		return "" // a status word or an id, not material
	}
	return strings.TrimSpace(best)
}
