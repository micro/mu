package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"mu/internal/service"
)

func promptCommands(prompt string, opts QueryOpts) ([]service.CommandCall, bool) {
	if !explicitCommand(prompt) && strings.TrimSpace(opts.Extra) != "" {
		return nil, false
	}
	return service.MatchCommandsFor(strings.TrimPrefix(strings.TrimSpace(prompt), "/"), filterServices(nativeServices(opts.Public), opts.Tools), !opts.Public)
}

func promptCommand(prompt string, opts QueryOpts) (service.CommandCall, bool) {
	calls, ok := promptCommands(prompt, opts)
	if !ok || len(calls) != 1 {
		return service.CommandCall{}, false
	}
	return calls[0], true
}

func executeCommands(ctx context.Context, account string, calls []service.CommandCall, opts QueryOpts) (string, error) {
	if len(calls) == 1 {
		return executeCommand(ctx, account, calls[0], opts)
	}
	// Hooks write to a shared response/record. Serialize them while the service
	// reads run concurrently, and publish the complete answer in request order.
	var mu sync.Mutex
	child := opts
	child.Stream.Token = nil
	child.Stream.ToolStart = func(r ToolRun) {
		mu.Lock()
		defer mu.Unlock()
		if opts.Stream.ToolStart != nil {
			opts.Stream.ToolStart(r)
		}
	}
	child.Stream.ToolEnd = func(r ToolRun) {
		mu.Lock()
		defer mu.Unlock()
		if opts.Stream.ToolEnd != nil {
			opts.Stream.ToolEnd(r)
		}
	}
	child.OnStep = func(s Step) {
		mu.Lock()
		defer mu.Unlock()
		if opts.OnStep != nil {
			opts.OnStep(s)
		}
	}
	results := make([]string, len(calls))
	errs := make([]error, len(calls))
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(i int, call service.CommandCall) {
			defer wg.Done()
			text, err := executeCommandRun(ctx, account, call, child, fmt.Sprintf("command-%d", i))
			if err != nil {
				errs[i] = err
				text = "Could not complete this request: " + err.Error()
			}
			results[i] = "### " + service.Label(call.Service+"_"+strings.ToLower(call.Method)) + "\n\n" + text
		}(i, call)
	}
	wg.Wait()
	failed := 0
	for _, err := range errs {
		if err != nil {
			failed++
		}
	}
	if failed == len(calls) {
		return "", errors.Join(errs...)
	}
	text := strings.Join(results, "\n\n")
	if opts.Stream.Token != nil {
		opts.Stream.Token(text)
	}
	return text, nil
}

// A known read outside the current scope is a refusal, not an invitation to
// spend a model turn trying the same unavailable operation.
func commandDenied(prompt string, opts QueryOpts) bool {
	if !explicitCommand(prompt) && strings.TrimSpace(opts.Extra) != "" {
		return false
	}
	if _, ok := promptCommands(prompt, opts); ok {
		return false
	}
	_, known := service.MatchCommandsFor(strings.TrimPrefix(strings.TrimSpace(prompt), "/"), service.Services(), true)
	return known
}

func explicitCommand(prompt string) bool {
	return strings.HasPrefix(strings.TrimSpace(prompt), "/")
}

func executeCommand(ctx context.Context, account string, call service.CommandCall, opts QueryOpts) (string, error) {
	return executeCommandRun(ctx, account, call, opts, "command")
}

func executeCommandRun(ctx context.Context, account string, call service.CommandCall, opts QueryOpts, id string) (string, error) {
	name := call.Service + "_" + strings.ToLower(call.Method)
	run := ToolRun{ID: id, Name: name, Label: toolLabel(name)}
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
	for _, key := range []string{"summary", "text", "events"} {
		if s, ok := result[key].(string); ok && s != "" {
			return s
		}
	}

	for _, key := range []string{"notes", "entries"} {
		if raw, exists := result[key]; exists {
			rows, _ := raw.([]any)
			if len(rows) == 0 {
				return "No results."
			}
			var lines []string
			escape := strings.NewReplacer("\\", "\\\\", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "<", "&lt;", ">", "&gt;", "`", "\\`", "\n", " ")
			for _, raw := range rows {
				row, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				title, _ := row["title"].(string)
				text, _ := row["text"].(string)
				if title != "" && text != "" {
					text = title + " — " + text
				} else if title != "" {
					text = title
				}
				if text != "" {
					lines = append(lines, "- "+escape.Replace(text))
				}
			}
			if len(lines) > 0 {
				return strings.Join(lines, "\n")
			}
		}
	}
	if len(result) == 1 {
		for _, key := range []string{"text", "summary"} {
			if _, exists := result[key]; exists {
				return ""
			}
		}
	}
	if items, ok := result["items"].([]any); ok {
		for _, raw := range items {
			if item, ok := raw.(map[string]any); ok {
				if link, ok := item["url"].(string); ok {
					u, err := url.Parse(link)
					if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
						return ""
					}
				}
			}
		}
	}
	// Other registered read responses still have a useful exact representation.
	// Do not execute a service successfully and then claim it returned nothing
	// merely because its field names differ from news and weather.
	if len(result) > 0 {
		b, err := json.MarshalIndent(result, "", "  ")
		if err == nil {
			return "```json\n" + string(b) + "\n```"
		}
	}
	return ""
}
