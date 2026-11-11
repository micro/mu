package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

func origin(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", fmt.Errorf("use a runtime origin without a path, credentials, query or fragment")
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return "", fmt.Errorf("use HTTPS (HTTP is allowed on loopback)")
	}
	return u.Scheme + "://" + u.Host, nil
}

func randomID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func transport() *http.Client {
	return &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func safeText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
}

type client struct {
	server, token, path string
	http                *http.Client
}

func (c *client) call(ctx context.Context, service, endpoint string, body, out any) error {
	token := c.token
	if token == "" {
		var err error
		token, err = credentials(ctx, c.path, c.server, c.http)
		if err != nil {
			return err
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	if len(raw) > 65536 {
		return fmt.Errorf("request exceeds 64 KiB")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.server+"/", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	id := randomID()
	for k, v := range map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + token, "Micro-Protocol": "mucp/1", "Micro-Service": service, "Micro-Endpoint": endpoint, "Micro-Id": id} {
		req.Header.Set(k, v)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 4*1024*1024+1))
	if err != nil {
		return err
	}
	if len(data) > 4*1024*1024 {
		return fmt.Errorf("response exceeds 4 MiB")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("runtime HTTP %d: %s", res.StatusCode, safeText(string(data)))
	}
	if res.Header.Get("Micro-Protocol") != "mucp/1" || res.Header.Get("Micro-Id") != id {
		return fmt.Errorf("invalid MUCP protocol or correlation header")
	}
	return json.Unmarshal(data, out)
}

type answer struct {
	Text    string `json:"text"`
	Content []struct {
		Fallback string `json:"fallback"`
	} `json:"content"`
}
type operation struct {
	raw    json.RawMessage
	ID     string  `json:"id"`
	Thread string  `json:"thread"`
	State  string  `json:"state"`
	Error  string  `json:"error"`
	Result *answer `json:"result"`
}

// Keep unknown fields and typed content intact for scripts using --json.
func (op *operation) UnmarshalJSON(data []byte) error {
	type view operation
	var decoded view
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*op = operation(decoded)
	op.raw = append(json.RawMessage(nil), data...)
	return nil
}

func (c *client) wait(ctx context.Context, op operation) (operation, error) {
	id := op.ID
	if id == "" {
		return op, fmt.Errorf("runtime returned no operation ID")
	}
	for {
		switch op.State {
		case "OPERATION_STATE_SUCCEEDED":
			if op.Result == nil {
				return op, fmt.Errorf("runtime returned no result")
			}
			return op, nil
		case "OPERATION_STATE_FAILED", "OPERATION_STATE_INTERRUPTED":
			return op, fmt.Errorf("%s", safeText(op.Error))
		case "OPERATION_STATE_QUEUED", "OPERATION_STATE_RUNNING":
		default:
			return op, fmt.Errorf("unknown operation state %q", op.State)
		}
		select {
		case <-ctx.Done():
			return op, ctx.Err()
		case <-time.After(2 * time.Second):
		}
		var next operation
		if err := c.call(ctx, "assistant", "Assistant.ReadOperation", map[string]string{"id": id}, &next); err != nil {
			return op, err
		}
		if next.ID != id || next.Thread != op.Thread {
			return op, fmt.Errorf("operation identity changed")
		}
		op = next
	}
}
