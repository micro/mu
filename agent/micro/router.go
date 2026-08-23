package micro

// Addressing an agent by name.
//
// There was a router here: a keyword table mapping "weather" and "BTC price"
// and "unread mail" to one of ten specialists, an LLM call for the prompts the
// table missed, and an orchestrator that ran two or three of them in parallel
// and merged the answers. It is gone with them. A router that can only ever
// return the one agent registered is not deciding anything, and a keyword table
// naming agents that do not exist is worse than none — it reads like routing
// and resolves to the fallback every time.
//
// What is left is the part that was never a guess. "@name" and "ask the name
// agent" are somebody saying which agent they want, and that still means
// something the moment an instance registers a second one, which Register is
// there for. Both consult the registry rather than a list of their own, so an
// address is only stripped when it names an agent that is actually here.

import "strings"

// MatchDirectAddress checks if the user explicitly addresses an agent.
// e.g. "ask the markets agent about ETH" or "@markets ETH price"
func MatchDirectAddress(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	lower := strings.ToLower(prompt)

	// "@agent" pattern
	if strings.HasPrefix(lower, "@") {
		parts := strings.Fields(lower)
		if len(parts) > 0 {
			id := strings.Trim(strings.TrimPrefix(parts[0], "@"), ` .,:;!?()[]{}<>"'`)
			if _, ok := Registry[id]; ok {
				return id
			}
		}
	}

	// "ask the X agent" pattern
	for _, prefix := range []string{"ask the ", "ask ", "use the ", "use "} {
		if strings.HasPrefix(lower, prefix) {
			rest := lower[len(prefix):]
			for id := range Registry {
				if strings.HasPrefix(rest, id+" agent") || strings.HasPrefix(rest, id+" ") || rest == id {
					return id
				}
			}
		}
	}

	return ""
}

// StripAddress removes the agent address prefix from a prompt.
func StripAddress(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	lower := strings.ToLower(prompt)

	if strings.HasPrefix(lower, "@") {
		parts := strings.SplitN(prompt, " ", 2)
		if len(parts) > 1 {
			return strings.TrimSpace(parts[1])
		}
		return prompt
	}

	for _, prefix := range []string{"ask the ", "ask ", "use the ", "use "} {
		if strings.HasPrefix(lower, prefix) {
			rest := prompt[len(prefix):]
			lowerRest := strings.ToLower(rest)
			for id := range Registry {
				stripped := ""
				if strings.HasPrefix(lowerRest, id+" agent about ") {
					stripped = rest[len(id)+len(" agent about "):]
				} else if strings.HasPrefix(lowerRest, id+" agent ") {
					stripped = rest[len(id)+len(" agent "):]
				} else if strings.HasPrefix(lowerRest, id+" about ") {
					stripped = rest[len(id)+len(" about "):]
				} else if strings.HasPrefix(lowerRest, id+" ") {
					stripped = rest[len(id)+1:]
				}
				if stripped != "" {
					return strings.TrimSpace(stripped)
				}
			}
		}
	}

	return prompt
}
