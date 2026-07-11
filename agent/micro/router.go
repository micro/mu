package micro

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

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

// keywordRoute handles obvious cases without an LLM call.
func keywordRoute(prompt string) []string {
	lower := strings.ToLower(prompt)

	// Single-domain keywords are checked in a fixed order so prompts that
	// contain more than one keyword route predictably instead of depending on
	// Go's randomized map iteration order.
	routes := []struct {
		keyword string
		ids     []string
	}{
		{keyword: "mail", ids: []string{"mail"}},
		{keyword: "email", ids: []string{"mail"}},
		{keyword: "inbox", ids: []string{"mail"}},
		{keyword: "unread", ids: []string{"mail"}},
		{keyword: "quran", ids: []string{"faith"}},
		{keyword: "hadith", ids: []string{"faith"}},
		{keyword: "reminder", ids: []string{"faith"}},
		{keyword: "surah", ids: []string{"faith"}},
		{keyword: "verse", ids: []string{"faith"}},
		{keyword: "coworking", ids: []string{"places"}},
		{keyword: "nearby", ids: []string{"places"}},
		{keyword: "restaurant", ids: []string{"places"}},
		{keyword: "cafe", ids: []string{"places"}},
		{keyword: "blog", ids: []string{"social"}},
		{keyword: "post", ids: []string{"social"}},
	}

	for _, route := range routes {
		if containsRouteTerm(lower, route.keyword) {
			return route.ids
		}
	}

	// Multi-signal detection
	hasWeather := containsAnyRouteTerm(lower, "weather", "forecast", "temperature")
	hasNews := containsAnyRouteTerm(lower, "news", "headline", "happening")
	hasMarkets := containsAnyRouteTerm(lower, "price", "market", "btc", "eth", "crypto")
	hasVideo := containsAnyRouteTerm(lower, "video", "watch", "youtube")
	hasSearch := containsAnyRouteTerm(lower, "search", "look up", "find out")
	hasApps := containsAnyRouteTerm(lower, "build me", "build an app", "create an app")

	var ids []string
	if hasWeather {
		ids = append(ids, "weather")
	}
	if hasNews {
		ids = append(ids, "news")
	}
	if hasMarkets {
		ids = append(ids, "markets")
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

func containsAnyRouteTerm(prompt string, terms ...string) bool {
	for _, term := range terms {
		if containsRouteTerm(prompt, term) {
			return true
		}
	}
	return false
}

func containsRouteTerm(prompt, term string) bool {
	if term == "" {
		return false
	}

	start := 0
	for {
		idx := strings.Index(prompt[start:], term)
		if idx == -1 {
			return false
		}
		idx += start
		end := idx + len(term)
		if isRouteStartBoundary(prompt, idx) && isRouteEndBoundary(prompt, end) {
			return true
		}
		start = idx + 1
	}
}

func isRouteStartBoundary(s string, idx int) bool {
	if idx <= 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(s[:idx])
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

func isRouteEndBoundary(s string, idx int) bool {
	if idx >= len(s) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(s[idx:])
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
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
