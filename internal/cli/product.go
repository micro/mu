package cli

// Hosted agent operations use the same HTTP resources as the web application.
// Service tools continue to use MCP. No model credentials are needed here.
import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type productCommand struct{ path, method, action, positional, flags, help string }

var productCommands = map[string]productCommand{
	"agent_create": {"/agents", "POST", "", "name", "name prompt tools", "mu agent create NAME --prompt INSTRUCTIONS --tools web,news"},
	"agent_list":   {"/agents", "GET", "", "", "", "mu agent list"},
	"work_submit":  {"/work", "POST", "", "prompt", "prompt agent thread", "mu work submit [--agent NAME] --prompt GOAL"},
	"work_get":     {"/work", "GET", "", "id", "id", "mu work get --id WORK_ID"},
	"work_list":    {"/work", "GET", "", "", "status", "mu work list [--status failed]"},
	"work_retry":   {"/work", "POST", "retry", "id", "id", "mu work retry --id WORK_ID"},
	"inbox_list":   {"/inbox", "GET", "", "", "", "mu inbox list"},
	"inbox_read":   {"/inbox", "GET", "", "id", "id", "mu inbox read --id THREAD_ID"},
}

func productName(command string, args []string) (string, []string) {
	if command == "agent" || command == "work" || command == "inbox" {
		if len(args) > 0 {
			command, args = command+"_"+args[0], args[1:]
		}
	}
	switch command {
	case "work_create":
		command = "work_submit"
	case "work_read":
		command = "work_get"
	}
	return command, args
}

func runProduct(command string, args []string, rc *ResolvedConfig) (int, bool) {
	name, args := productName(command, args)
	spec, ok := productCommands[name]
	if !ok {
		return 0, false
	}
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			fmt.Println(spec.help)
			return 0, true
		}
	}
	payload, err := productArguments(spec, args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, spec.help)
		return 2, true
	}
	if err = rc.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2, true
	}
	result, err := NewClient(rc).productRequest(spec, payload)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1, true
	}
	if err = Format(os.Stdout, string(result), rc); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1, true
	}
	return 0, true
}

// Keep IDs and prompts as strings, even when they look numeric. Mutations must
// never be retried just to discover argument types.
func productArguments(spec productCommand, args []string) (map[string]any, error) {
	allowed := map[string]bool{}
	for _, key := range strings.Fields(spec.flags) {
		allowed[key] = true
	}
	out := map[string]any{}
	var words []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			words = append(words, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") {
			words = append(words, a)
			continue
		}
		flag, value, inline := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		if !allowed[flag] {
			return nil, fmt.Errorf("unknown flag: %s", a)
		}
		if !inline {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return nil, fmt.Errorf("--%s needs a value", flag)
			}
			i++
			value = args[i]
		}
		if _, exists := out[flag]; exists {
			return nil, fmt.Errorf("--%s was supplied twice", flag)
		}
		out[flag] = value
	}
	if len(words) > 0 {
		if spec.positional == "" {
			return nil, fmt.Errorf("unexpected arguments")
		}
		if _, exists := out[spec.positional]; exists {
			return nil, fmt.Errorf("supply %s once", spec.positional)
		}
		out[spec.positional] = strings.Join(words, " ")
	}
	if spec.positional != "" && (out[spec.positional] == nil || strings.TrimSpace(fmt.Sprint(out[spec.positional])) == "") {
		return nil, fmt.Errorf("%s is required", spec.positional)
	}
	if spec.path == "/agents" && spec.method == "POST" {
		prompt, _ := out["prompt"].(string)
		tools, _ := out["tools"].(string)
		if strings.TrimSpace(prompt) == "" || strings.TrimSpace(tools) == "" {
			return nil, fmt.Errorf("provide --prompt and --tools")
		}
		var services []string
		for _, s := range strings.Split(tools, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				return nil, fmt.Errorf("--tools needs comma-separated service names")
			}
			services = append(services, s)
		}
		delete(out, "tools")
		out["services"] = services
	}
	if spec.action != "" {
		out["action"] = spec.action
	}
	return out, nil
}

func (c *Client) productRequest(spec productCommand, args map[string]any) ([]byte, error) {
	endpoint := c.URL + spec.path
	var body io.Reader
	if spec.method == "GET" {
		query := url.Values{}
		for k, v := range args {
			if k == "id" {
				endpoint += "/" + url.PathEscape(fmt.Sprint(v))
				continue
			}
			query.Set(k, fmt.Sprint(v))
		}
		if len(query) > 0 {
			endpoint += "?" + query.Encode()
		}
	} else {
		encoded, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(spec.method, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	client := *c.HTTP
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request not confirmed; check the server state before retrying: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.StatusCode == 401 {
			return nil, fmt.Errorf("not signed in — run mu login or set MU_TOKEN")
		}
		if message := httpErrorMessage(data); message != "" {
			return nil, fmt.Errorf("%s", message)
		}
		return nil, fmt.Errorf("HTTP %d from %s", response.StatusCode, spec.path)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("expected JSON from %s; check your server URL", spec.path)
	}
	return data, nil
}
