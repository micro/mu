package micro

import (
	"encoding/json"
	"fmt"
	"strings"

	"mu/internal/ai"
)

// editPrompt asks the model to modify an existing spec rather than invent one.
//
// This is the difference between editing and regenerating: the model is shown
// the current spec and told to return it changed, so anything the instruction
// does not mention survives verbatim. Asking for a fresh spec from an updated
// description silently drops whatever was not restated — which is exactly the
// failure users notice when they ask for one small change and lose a field.
const editPrompt = systemPrompt + `

You are now EDITING an existing app rather than creating one.

You will be given the current spec as JSON, followed by an instruction.
Return the complete updated spec as a single JSON object.

Editing rules:
- Preserve everything the instruction does not ask you to change: the title,
  the emoji, existing fields, items and counters, and their order.
- Apply only the requested change.
- You may change "type" only if the instruction clearly calls for a different
  kind of app; the result must still be a valid spec of that type.
- Output a single JSON object and nothing else.`

// Edit applies a natural-language instruction to an existing spec and returns
// the updated one. Like Generate, it validates and retries with the error fed
// back, so the result always renders.
func Edit(current *Spec, instruction string) (*Spec, error) {
	if current == nil {
		return nil, fmt.Errorf("no current spec to edit")
	}
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return nil, fmt.Errorf("say what you want changed")
	}

	b, err := json.Marshal(current)
	if err != nil {
		return nil, fmt.Errorf("could not read the current app: %w", err)
	}

	base := fmt.Sprintf("Current spec:\n%s\n\nInstruction: %s", b, instruction)
	question := base

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		raw, err := ai.Ask(&ai.Prompt{
			System:    editPrompt,
			Question:  question,
			Model:     ai.ModelDeepSeekFlash,
			Caller:    "micro-app-edit",
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

		lastErr = perr
		question = fmt.Sprintf("%s\n\nYour previous answer was invalid: %s\nReturn a corrected JSON spec.", base, perr)
	}
	return nil, fmt.Errorf("could not apply that change: %w", lastErr)
}
