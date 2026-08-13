// Package text is language work as a capability: summarise it, pull structure
// out of it, sort it into a label, put it in another language.
//
// Named for what it is about rather than for what runs it. "ai" is not a domain
// — it is how these happen to be implemented, and this repository does not put
// implementation vocabulary in a service name any more than it puts it in the
// product. A caller wants something done to some text; that the doing involves
// a model is our business.
//
// Deliberately not scoped. Nothing here is anybody's private data — text goes
// in, an answer comes back, nothing is kept — which is what lets a paying agent
// with no account use every one of these. That property is the point: these are
// the tools with a real marginal cost and no reputation attached, so they are
// exactly what an anonymous caller should be able to buy.
//
// What is sold is not the model. Anyone can reach a model. What is sold is a
// fixed price for a finished piece of work, payable in USDC with no signup —
// three pence to turn a page into JSON, known before the call rather than
// totted up per token afterwards.
package text

import (
	"fmt"
	"strings"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/quota"
	"mu/internal/service"
)

// maxInput is the most text one call will consider, in characters.
//
// Our cost varies with input length and the price does not: a two-hundred word
// summary and a fifty-thousand word one both cost the caller the same. Something
// has to give, and a documented cap is kinder than a surprise — a caller who
// knows the limit can split the work, where a caller who is billed by the token
// only finds out afterwards.
//
// Roughly 30k characters is about 8k tokens, which every model here handles
// comfortably and no honest single call exceeds.
const maxInput = 30000

// clip trims input to the cap and says whether it had to.
func clip(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) <= maxInput {
		return s, false
	}
	return s[:maxInput], true
}

// note is appended to an answer built from clipped input, so a caller is never
// told a truncated answer is a complete one.
const note = "\n\n(Only the first %d characters were read; the input was %d.)"

// ask runs a prompt and returns the answer.
//
// Every method here is the same shape — a system prompt, some text, one
// answer — so they share one path rather than four almost-identical ones.
func ask(system, question, model string) (string, error) {
	out, err := ai.Ask(&ai.Prompt{
		System:   system,
		Question: question,
		Model:    model,
		Caller:   "text",
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("the model returned nothing")
	}
	return strings.TrimSpace(out), nil
}

// withNote appends the truncation note when input was clipped.
func withNote(out string, clipped bool, original int) string {
	if !clipped {
		return out
	}
	return out + fmt.Sprintf(note, maxInput, original)
}

// Load registers the service.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("text", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:        "text",
	Handler:     new(Server),
	Description: "Language work: summarise, extract structure, classify, translate",
	Page:        "/text",
	Icon:        "docs.svg",
	Endpoints: map[string]service.Endpoint{
		"Summarise": {
			Aliases: []string{"text_summarize"},
			Doc: "Summarise text into a few sentences. Pass style=bullets for a list, " +
				"or a sentence count. Capped at 30,000 characters",
			Cost: quota.OpTextSummarise,
		},
		"Extract": {
			Doc: "Turn text into JSON matching a schema you give. Pass the fields you want " +
				"as a JSON schema or a plain description; returns JSON only. Capped at 30,000 characters",
			Cost: quota.OpTextExtract,
		},
		"Classify": {
			Doc: "Sort text into one of the labels you give, with a confidence. " +
				"For routing, triage and moderation. Capped at 30,000 characters",
			Cost: quota.OpTextClassify,
		},
		"Translate": {
			Doc: "Translate text into another language, preserving formatting. " +
				"Capped at 30,000 characters",
			Cost: quota.OpTextTranslate,
		},
	},
}
