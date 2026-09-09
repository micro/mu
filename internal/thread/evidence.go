package thread

import (
	"context"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Evidence keeps bounded tool observations separate from spoken messages.
type Evidence struct {
	Tool      string    `json:"tool"`
	Service   string    `json:"service"`
	Arguments string    `json:"arguments"`
	Result    string    `json:"result"`
	At        time.Time `json:"at"`
	Expires   time.Time `json:"expires"`
	Truncated bool      `json:"truncated,omitempty"`
}

func KeepEvidence(account, id string, e Evidence) {
	if account == "" || len(e.Result) == 0 {
		return
	}
	ensure()
	mu.Lock()
	defer mu.Unlock()
	t := threads[id]
	if t == nil || t.Account != account {
		return
	}
	if len(e.Result) > 8000 {
		e.Result = e.Result[:8000]
		e.Truncated = true
	}
	if len(e.Arguments) > 2000 {
		return
	}
	// Copy-on-write: snapshots already handed to the disk writer stay immutable.
	next := make([]Evidence, 0, 12)
	for _, old := range t.Evidence {
		if old.Tool == e.Tool && old.Arguments == e.Arguments {
			continue
		}
		if old.Expires.Before(time.Now()) {
			continue
		}
		next = append(next, old)
	}
	next = append(next, e)
	if len(next) > 12 {
		next = next[len(next)-12:]
	}
	t.Evidence = next
	save()
}

func EvidenceFor(account, id string, allowed []string, now time.Time) []Evidence {
	if account == "" {
		return nil
	}
	ensure()
	mu.RLock()
	defer mu.RUnlock()
	t := threads[id]
	if t == nil || t.Account != account {
		return nil
	}
	permit := map[string]bool{}
	for _, s := range allowed {
		permit[s] = true
	}
	var out []Evidence
	for i := len(t.Evidence) - 1; i >= 0; i-- {
		e := t.Evidence[i]
		if permit[e.Service] && e.Expires.After(now) {
			out = append(out, e)
		}
		if len(out) == 3 {
			break
		}
	}
	return out
}

// Recall retrieves whole-message excerpts, with roles and thread provenance.
// It searches only the caller's agent conversations, excluding mail and rooms:
// permission to recall personal dialogue does not grant access to other tools.
func Recall(ctx context.Context, account, current string, terms []string) []Hit {
	if account == "" || len(terms) == 0 {
		return nil
	}
	ensure()
	mu.RLock()
	defer mu.RUnlock()
	type ranked struct {
		hit   Hit
		score int
	}
	var best []ranked
	for id, t := range owned[account] {
		if id == current || t.Client != "web" {
			continue
		}
		for _, m := range messages[id] {
			if ctx.Err() != nil {
				return nil
			}
			words := map[string]bool{}
			// QueryTerms filters grammar and caps at eight, so use word boundaries
			// over the complete text here instead of extracting query terms again.
			for _, word := range strings.FieldsFunc(strings.ToLower(m.Text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
				words[word] = true
			}
			score := 0
			for _, term := range terms {
				if words[term] {
					score++
				}
			}
			if score != len(terms) {
				continue
			}
			copy := *m
			if len(copy.Text) > 1600 {
				copy.Text = copy.Text[:1600] + " … [excerpt]"
			}
			best = append(best, ranked{Hit{Message: copy, Client: t.Client, Subject: t.Subject, Where: InText}, score})
			sort.Slice(best, func(i, j int) bool {
				if best[i].score != best[j].score {
					return best[i].score > best[j].score
				}
				return best[i].hit.At.After(best[j].hit.At)
			})
			if len(best) > 3 {
				best = best[:3]
			}
		}
	}
	var out []Hit
	for _, b := range best {
		out = append(out, b.hit)
	}
	return out
}
