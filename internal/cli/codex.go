package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// This is a readiness probe, not a model provider. No thread, turn or tool call
// is started, and credentials and account email are never printed.
func runCodex(args []string) int {
	if len(args) != 1 || args[0] != "status" {
		fmt.Fprintln(os.Stderr, "Usage: mu codex status")
		return 2
	}
	binary, err := exec.LookPath("codex")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Codex CLI is not installed. Install it, run codex login, then run mu codex status.")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "app-server", "--listen", "stdio://")
	in, err := cmd.StdinPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err = cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { in.Close(); cancel(); cmd.Wait() }()
	status, err := probeCodex(out, in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not inspect Codex app-server:", err)
		return 1
	}
	if status.AccountType == "" {
		fmt.Println("Codex is not signed in. Run codex login.")
		return 1
	}
	fmt.Println("Codex account:", status.AccountType)
	fmt.Println("Available models:")
	for _, m := range status.Models {
		fmt.Printf("  %s (%s)\n", m.Model, m.DisplayName)
	}
	fmt.Println("Readiness check only; Micro's model provider has not changed.")
	return 0
}

type codexModel struct {
	Model       string `json:"model"`
	DisplayName string `json:"displayName"`
}
type codexStatus struct {
	AccountType string
	Models      []codexModel
}

func probeCodex(r io.Reader, w io.Writer) (codexStatus, error) {
	var status codexStatus
	enc := json.NewEncoder(w)
	scan := bufio.NewScanner(r)
	scan.Buffer(make([]byte, 4096), 4<<20)
	id := 0
	call := func(method string, params any, result any) error {
		id++
		if err := enc.Encode(map[string]any{"id": id, "method": method, "params": params}); err != nil {
			return err
		}
		for scan.Scan() {
			var msg struct {
				ID     *int            `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  *struct {
					Code int `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(scan.Bytes(), &msg); err != nil {
				return fmt.Errorf("invalid app-server response")
			}
			if msg.ID == nil {
				continue
			}
			if *msg.ID != id {
				return fmt.Errorf("unexpected app-server response id")
			}
			if msg.Error != nil {
				return fmt.Errorf("%s failed (code %d)", method, msg.Error.Code)
			}
			if result != nil {
				return json.Unmarshal(msg.Result, result)
			}
			return nil
		}
		if err := scan.Err(); err != nil {
			return err
		}
		return io.ErrUnexpectedEOF
	}
	if err := call("initialize", map[string]any{"clientInfo": map[string]string{"name": "micro", "title": "Micro", "version": "1"}}, nil); err != nil {
		return status, err
	}
	if err := enc.Encode(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
		return status, err
	}
	var account struct {
		Account *struct {
			Type string `json:"type"`
		} `json:"account"`
	}
	if err := call("account/read", map[string]bool{"refreshToken": false}, &account); err != nil {
		return status, err
	}
	if account.Account == nil {
		return status, nil
	}
	status.AccountType = account.Account.Type
	cursor := ""
	seen := map[string]bool{}
	for {
		params := map[string]any{"limit": 100, "includeHidden": false}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var page struct {
			Data       []codexModel `json:"data"`
			NextCursor *string      `json:"nextCursor"`
		}
		if err := call("model/list", params, &page); err != nil {
			return status, err
		}
		status.Models = append(status.Models, page.Data...)
		if page.NextCursor == nil || *page.NextCursor == "" {
			break
		}
		cursor = *page.NextCursor
		if seen[cursor] {
			return status, fmt.Errorf("repeated model cursor")
		}
		seen[cursor] = true
	}
	return status, nil
}
