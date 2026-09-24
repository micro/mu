// Package codex adapts an isolated Codex App Server to Micro's model interface.
package codex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

type message struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code int `json:"code"`
	} `json:"error"`
}

type rpc struct {
	scan   *bufio.Scanner
	out    *json.Encoder
	id     int
	handle func(message) error
}

func newRPC(r io.Reader, w io.Writer) *rpc {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 4096), 8<<20)
	return &rpc{scan: s, out: json.NewEncoder(w)}
}

func (c *rpc) read() (message, error) {
	var m message
	if !c.scan.Scan() {
		if err := c.scan.Err(); err != nil {
			return m, fmt.Errorf("codex transport: %w", err)
		}
		return m, io.ErrUnexpectedEOF
	}
	if json.Unmarshal(c.scan.Bytes(), &m) != nil {
		return m, fmt.Errorf("invalid Codex response")
	}
	return m, nil
}

func (c *rpc) call(method string, params, result any) error {
	c.id++
	id := c.id
	if err := c.out.Encode(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	for {
		m, err := c.read()
		if err != nil {
			return err
		}
		if m.Method != "" {
			if err := c.dispatch(m); err != nil {
				return err
			}
			continue
		}
		if string(m.ID) != fmt.Sprint(id) {
			return fmt.Errorf("unexpected Codex response id")
		}
		// Error text may contain credentials or private provider state. Never expose it.
		if m.Error != nil {
			return fmt.Errorf("Codex %s failed (code %d)", method, m.Error.Code)
		}
		if result != nil {
			return json.Unmarshal(m.Result, result)
		}
		return nil
	}
}

func (c *rpc) dispatch(m message) error {
	if c.handle != nil {
		return c.handle(m)
	}
	if len(m.ID) > 0 {
		return fmt.Errorf("unexpected Codex server request")
	}
	return nil
}

func (c *rpc) initialize() error {
	if err := c.call("initialize", map[string]any{"clientInfo": map[string]string{"name": "micro", "version": "1"}, "capabilities": map[string]bool{"experimentalApi": true}}, nil); err != nil {
		return err
	}
	return c.out.Encode(map[string]any{"method": "initialized", "params": map[string]any{}})
}
