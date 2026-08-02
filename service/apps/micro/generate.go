package micro

import (
	"encoding/json"
	"fmt"
	"strings"

	"mu/internal/ai"
)

// systemPrompt instructs the model to emit only a micro-app spec as JSON.
//
// The model never writes markup or code — it picks one of a few known shapes
// and fills in the fields. That is the whole point: a tiny, checkable output
// the renderer can always turn into a working app.
const systemPrompt = `You design small single-purpose "micro apps". Given a description, you output ONLY a JSON spec — no prose, no markdown, no code fences.

Pick exactly one of three types:

1. "tracker" — a list the user adds entries to, optionally totalling a number.
   {"type":"tracker","title":"Expenses","emoji":"💸","fields":[{"name":"Item","type":"text"},{"name":"Amount","type":"number"},{"name":"Date","type":"date"}],"sum":"Amount"}
   - "fields" is required; each field "type" is "text", "number" or "date".
   - "sum" is optional and must name one of the number fields to total.

2. "checklist" — a list of checkable items.
   {"type":"checklist","title":"Packing List","emoji":"🧳","items":["Passport","Charger","Toothbrush"]}
   - "items" is required and must be non-empty.

3. "counter" — one or more +/- tallies.
   {"type":"counter","title":"Water Intake","emoji":"💧","counters":[{"label":"Glasses","step":1}]}
   - "counters" is required; each needs a "label" and optional integer "step" (default 1).

Rules:
- Output a single JSON object and nothing else.
- Always include a short "title" and a fitting "emoji".
- Choose the type that best fits the description; prefer the simplest that works.`

// Generate turns a natural-language description into a validated Spec.
//
// It asks the model for a spec, validates it, and if validation fails feeds the
// error back and retries — a short repair loop. Because the output space is
// tiny and checkable, this converges fast and the result always renders.
func Generate(description string) (*Spec, error) {
	description = strings.TrimSpace(description)
	if description == "" {
		return nil, fmt.Errorf("description is required")
	}

	question := description
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		raw, err := ai.Ask(&ai.Prompt{
			System:    systemPrompt,
			Question:  question,
			Model:     ai.ModelDeepSeekFlash,
			Caller:    "micro-app",
			MaxTokens: 800,
		})
		if err != nil {
			return nil, fmt.Errorf("ai request failed: %w", err)
		}

		spec, perr := parseSpec(raw)
		if perr == nil {
			perr = spec.Validate()
		}
		if perr == nil {
			return spec, nil
		}

		// Feed the failure back and ask for a corrected spec.
		lastErr = perr
		question = fmt.Sprintf("%s\n\nYour previous answer was invalid: %s\nReturn a corrected JSON spec.", description, perr)
	}
	return nil, fmt.Errorf("could not generate a valid spec: %w", lastErr)
}

// parseSpec extracts a Spec from a model response, tolerating code fences and
// surrounding prose by isolating the outermost JSON object.
func parseSpec(raw string) (*Spec, error) {
	s := stripFences(raw)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("no JSON object in response")
	}
	s = s[start : end+1]
	var spec Spec
	if err := json.Unmarshal([]byte(s), &spec); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return &spec, nil
}

// stripFences removes a leading/trailing markdown code fence if present.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}
