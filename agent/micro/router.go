package micro

import (
	"encoding/json"
	"fmt"
	"strings"

	"mu/internal/ai"
)

// Route decides which agent(s) should handle a query.
func Route(prompt string) []string {
	// Check for direct addressing first
	if id := MatchDirectAddress(prompt); id != "" {
		return []string{id}
	}

	// Quick keyword matching for obvious cases (no LLM needed)
	if ids := keywordRoute(prompt); len(ids) > 0 {
		return ids
	}

	// LLM-based routing for ambiguous queries
	return llmRoute(prompt)
}

// MatchDirectAddress checks if the user explicitly addresses an agent.
// e.g. "ask the markets agent about ETH" or "@markets ETH price"
func MatchDirectAddress(prompt string) string {
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

// keywordRoute handles obvious cases without an LLM call.
func keywordRoute(prompt string) []string {
	lower := strings.ToLower(prompt)

	// Single-domain keywords
	routes := map[string][]string{
		"mail":       {"mail"},
		"email":      {"mail"},
		"inbox":      {"mail"},
		"unread":     {"mail"},
		"quran":      {"faith"},
		"hadith":     {"faith"},
		"reminder":   {"faith"},
		"surah":      {"faith"},
		"verse":      {"faith"},
		"coworking":  {"places"},
		"nearby":     {"places"},
		"restaurant": {"places"},
		"cafe":       {"places"},
		"blog":       {"social"},
		"post":       {"social"},
	}

	for keyword, ids := range routes {
		if strings.Contains(lower, keyword) {
			return ids
		}
	}

	// Multi-signal detection
	hasWeather := strings.Contains(lower, "weather") || strings.Contains(lower, "forecast") || strings.Contains(lower, "temperature")
	hasNews := strings.Contains(lower, "news") || strings.Contains(lower, "headline") || strings.Contains(lower, "happening")
	hasMarkets := strings.Contains(lower, "price") || strings.Contains(lower, "market") || strings.Contains(lower, "btc") || strings.Contains(lower, "eth") || strings.Contains(lower, "crypto")
	hasTrade := strings.Contains(lower, "swap") || strings.Contains(lower, "trade") || strings.Contains(lower, "buy") || strings.Contains(lower, "sell") || strings.Contains(lower, "strategy")
	hasVideo := strings.Contains(lower, "video") || strings.Contains(lower, "watch") || strings.Contains(lower, "youtube")
	hasSearch := strings.Contains(lower, "search") || strings.Contains(lower, "look up") || strings.Contains(lower, "find out")
	hasApps := strings.Contains(lower, "build me") || strings.Contains(lower, "build an app") || strings.Contains(lower, "create an app")

	var ids []string
	if hasWeather {
		ids = append(ids, "weather")
	}
	if hasNews {
		ids = append(ids, "news")
	}
	if hasMarkets && !hasTrade {
		ids = append(ids, "markets")
	}
	if hasTrade {
		ids = append(ids, "trade")
	}
	if hasVideo {
		ids = append(ids, "video")
	}
	if hasSearch && len(ids) == 0 {
		ids = append(ids, "search")
	}
	if hasApps {
		ids = append(ids, "apps")
	}

	if len(ids) > 3 {
		ids = ids[:3]
	}
	return ids
}

// llmRoute uses the background model to classify the query.
func llmRoute(prompt string) []string {
	var agentList strings.Builder
	for id, a := range Registry {
		if id == "micro" {
			continue
		}
		agentList.WriteString(fmt.Sprintf("- %s: %s\n", id, a.Description))
	}

	result, err := ai.Ask(&ai.Prompt{
		System: "You are a routing agent. Given a user message, decide which specialist(s) should handle it.\n\n" +
			"Available agents:\n" + agentList.String() + "\n" +
			"Output ONLY a JSON array of agent IDs. Use at most 3. If unsure or general conversation, output [\"micro\"].",
		Question: prompt,
		Model:    ai.BackgroundModel(),
		Priority: ai.PriorityHigh,
		Caller:   "agent-router",
	})
	if err != nil {
		return []string{"micro"}
	}

	var ids []string
	result = strings.TrimSpace(result)
	start := strings.Index(result, "[")
	end := strings.LastIndex(result, "]")
	if start >= 0 && end > start {
		json.Unmarshal([]byte(result[start:end+1]), &ids)
	}

	if len(ids) == 0 {
		return []string{"micro"}
	}

	return validateAgentIDs(ids)
}

func validateAgentIDs(ids []string) []string {
	valid := make([]string, 0, min(len(ids), 3))
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		if _, ok := Registry[id]; !ok {
			continue
		}
		seen[id] = true
		valid = append(valid, id)
		if len(valid) == 3 {
			break
		}
	}
	if len(valid) == 0 {
		return []string{"micro"}
	}
	return valid
}
