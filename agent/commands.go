package agent

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"mu/internal/service"
)

func promptCommand(prompt string, opts QueryOpts) (service.CommandCall, bool) {
	// Explicit commands ignore attached prose; inferred phrases retain its context.
	if !explicitCommand(prompt) && strings.TrimSpace(opts.Extra) != "" {
		return service.CommandCall{}, false
	}
	return service.MatchCommandFor(strings.TrimPrefix(strings.TrimSpace(prompt), "/"), filterServices(nativeServices(opts.Public), opts.Tools), !opts.Public)
}

func explicitCommand(prompt string) bool {
	return strings.HasPrefix(strings.TrimSpace(prompt), "/")
}

func executeCommand(ctx context.Context, account string, call service.CommandCall, opts QueryOpts) (string, error) {
	name := call.Service + "_" + strings.ToLower(call.Method)
	run := ToolRun{ID: "command", Name: name, Label: toolLabel(name)}
	if opts.Stream.ToolStart != nil {
		opts.Stream.ToolStart(run)
	}
	start := time.Now()
	result, err := service.CallDynamic(service.WithAccount(ctx, account), call.Service, call.Method, call.Args)
	text := ""
	if err == nil {
		text = commandText(result)
		if strings.TrimSpace(text) == "" {
			err = fmt.Errorf("returned no result")
		}
	}
	if opts.Stream.ToolEnd != nil {
		opts.Stream.ToolEnd(run)
	}
	if opts.OnStep != nil {
		opts.OnStep(Step{Tool: name, Args: call.Args, OK: err == nil, Took: time.Since(start)})
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", call.Service, err)
	}
	if opts.Stream.Token != nil {
		opts.Stream.Token(text)
	}
	return text, nil
}

// Prefer structured links to the instruction-bearing prose supplied to models.
func commandText(result map[string]any) string {
	if items, ok := result["items"].([]any); ok && len(items) > 0 {
		var b strings.Builder
		escape := strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]", "*", "\\*", "_", "\\_", "<", "&lt;", ">", "&gt;", "\n", " ")
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			title, _ := item["title"].(string)
			link, _ := item["url"].(string)
			if title == "" {
				continue
			}
			u, err := url.Parse(link)
			if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
				continue
			}
			fmt.Fprintf(&b, "- %s — <%s>\n", escape.Replace(title), strings.NewReplacer("<", "%3C", ">", "%3E", "\n", "%0A", "\r", "%0D").Replace(u.String()))
		}
		if b.Len() > 0 {
			return b.String()
		}
	}
	for _, key := range []string{"summary", "text"} {
		if s, ok := result[key].(string); ok && s != "" {
			return s
		}
	}

	return ""
}
