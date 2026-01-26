package flow

import (
	"fmt"
	"regexp"
	"strings"
)

// Pattern matchers for flow syntax
var (
	// Schedule patterns: "every day at 7am", "every hour", "every monday at 9am"
	schedulePattern = regexp.MustCompile(`^every\s+(.+?)(?:\s+at\s+(\d{1,2}(?:am|pm)?))?\s*:?\s*$`)

	// Variable patterns: "save as myvar", "store as myvar"
	saveAsPattern = regexp.MustCompile(`^(?:save|store)\s+as\s+(\w+)$`)
	
	// Use variable: patterns containing $varname
	varPattern = regexp.MustCompile(`\$(\w+)`)

	// Conditional: "if X > Y then..."
	conditionalPattern = regexp.MustCompile(`^if\s+(.+?)\s+(>|<|>=|<=|==|!=)\s+(.+?)\s+then\s*:?$`)

	// Tool patterns - maps natural language to tool calls
	toolPatterns = []struct {
		pattern *regexp.Regexp
		tool    string
		args    func([]string) map[string]string
	}{
		// get reminder / get today's reminder
		{regexp.MustCompile(`^get\s+(?:today'?s?\s+)?reminder$`), "reminder.today", func(m []string) map[string]string { return nil }},

		// search news for "query"
		{regexp.MustCompile(`^search\s+news\s+for\s+["'](.+?)["']$`), "news.search", func(m []string) map[string]string { return map[string]string{"query": m[1]} }},

		// get news about "query"
		{regexp.MustCompile(`^get\s+news\s+about\s+["'](.+?)["']$`), "news.search", func(m []string) map[string]string { return map[string]string{"query": m[1]} }},

		// get headlines
		{regexp.MustCompile(`^get\s+(?:news\s+)?headlines$`), "news.headlines", func(m []string) map[string]string { return nil }},

		// email to me / email to "address" with subject "subject"
		{regexp.MustCompile(`^email\s+to\s+me(?:\s+with\s+subject\s+["'](.+?)["'])?$`), "mail.send", func(m []string) map[string]string {
			args := map[string]string{"to": "me"}
			if len(m) > 1 && m[1] != "" {
				args["subject"] = m[1]
			}
			return args
		}},
		{regexp.MustCompile(`^email\s+to\s+["'](.+?)["'](?:\s+with\s+subject\s+["'](.+?)["'])?$`), "mail.send", func(m []string) map[string]string {
			args := map[string]string{"to": m[1]}
			if len(m) > 2 && m[2] != "" {
				args["subject"] = m[2]
			}
			return args
		}},

		// get price of "BTC" / get btc price
		{regexp.MustCompile(`^get\s+(?:price\s+of\s+)?["']?([A-Za-z]+)["']?\s+price$`), "markets.get_price", func(m []string) map[string]string { return map[string]string{"symbol": strings.ToUpper(m[1])} }},
		{regexp.MustCompile(`^get\s+price\s+of\s+["']?([A-Za-z]+)["']?$`), "markets.get_price", func(m []string) map[string]string { return map[string]string{"symbol": strings.ToUpper(m[1])} }},

		// save note "content"
		{regexp.MustCompile(`^save\s+note\s+["'](.+?)["']$`), "notes.create", func(m []string) map[string]string { return map[string]string{"content": m[1]} }},

		// get balance / check balance
		{regexp.MustCompile(`^(?:get|check)\s+(?:my\s+)?balance$`), "wallet.balance", func(m []string) map[string]string { return nil }},

		// summarize (special - applies to previous result)
		{regexp.MustCompile(`^summarize$`), "summarize", func(m []string) map[string]string { return nil }},
	}
)

// Parse converts flow source text to a structured ParsedFlow
func Parse(source string) (*ParsedFlow, error) {
	lines := strings.Split(source, "\n")
	var cleanLines []string

	for _, line := range lines {
		// Trim and skip empty/comment lines
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cleanLines = append(cleanLines, line)
	}

	if len(cleanLines) == 0 {
		return nil, fmt.Errorf("empty flow")
	}

	result := &ParsedFlow{
		Trigger: "manual",
		Steps:   []*Step{},
	}

	startIdx := 0

	// Check if first line is a schedule trigger
	firstLine := strings.ToLower(cleanLines[0])
	if matches := schedulePattern.FindStringSubmatch(firstLine); matches != nil {
		result.Trigger = "schedule"
		result.Cron = parseCron(matches[1], matches[2])
		startIdx = 1
	}

	// Parse remaining lines as steps
	for i := startIdx; i < len(cleanLines); i++ {
		line := cleanLines[i]

		// Handle "then" prefix
		line = strings.TrimPrefix(line, "then ")
		line = strings.TrimSpace(line)

		step, err := parseStep(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %v", i+1, err)
		}
		result.Steps = append(result.Steps, step)
	}

	if len(result.Steps) == 0 {
		return nil, fmt.Errorf("no steps in flow")
	}

	return result, nil
}

// parseStep converts a single line to a Step
func parseStep(line string) (*Step, error) {
	lower := strings.ToLower(line)

	// Check for "save as varname" modifier at end
	var saveAs string
	if idx := strings.Index(lower, " save as "); idx > 0 {
		saveAs = strings.TrimSpace(lower[idx+9:])
		lower = strings.TrimSpace(lower[:idx])
		line = strings.TrimSpace(line[:idx])
	}

	// Check for "save as varname" as standalone (saves previous result)
	if matches := saveAsPattern.FindStringSubmatch(lower); matches != nil {
		return &Step{
			Tool:   "var.save",
			Args:   map[string]string{"name": matches[1]},
			SaveAs: matches[1],
		}, nil
	}

	for _, tp := range toolPatterns {
		if matches := tp.pattern.FindStringSubmatch(lower); matches != nil {
			return &Step{
				Tool:   tp.tool,
				Args:   tp.args(matches),
				SaveAs: saveAs,
			}, nil
		}
	}

	return nil, fmt.Errorf("unknown command: %s", line)
}

// parseCron converts natural schedule to cron-like format
func parseCron(interval, timeStr string) string {
	// Simple mapping - could be expanded
	interval = strings.ToLower(strings.TrimSpace(interval))
	timeStr = strings.ToLower(strings.TrimSpace(timeStr))

	hour := "8" // default 8am
	if timeStr != "" {
		// Parse "7am", "9pm", etc.
		if strings.HasSuffix(timeStr, "pm") {
			h := strings.TrimSuffix(timeStr, "pm")
			if h != "12" {
				hour = fmt.Sprintf("%d", atoi(h)+12)
			} else {
				hour = "12"
			}
		} else if strings.HasSuffix(timeStr, "am") {
			hour = strings.TrimSuffix(timeStr, "am")
		} else {
			hour = timeStr
		}
	}

	switch interval {
	case "day", "daily":
		return fmt.Sprintf("0 %s * * *", hour)
	case "hour", "hourly":
		return "0 * * * *"
	case "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday":
		dayNum := map[string]string{
			"sunday": "0", "monday": "1", "tuesday": "2", "wednesday": "3",
			"thursday": "4", "friday": "5", "saturday": "6",
		}[interval]
		return fmt.Sprintf("0 %s * * %s", hour, dayNum)
	case "morning":
		return "0 8 * * *"
	case "evening":
		return "0 18 * * *"
	default:
		return fmt.Sprintf("0 %s * * *", hour) // Default to daily
	}
}

func atoi(s string) int {
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}
