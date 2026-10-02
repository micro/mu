package privacy

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	gmai "go-micro.dev/v6/model"
)

// New protects both initial requests and the provider's internal tool loop.
// Authorization still runs in the original handler, after argument restoration.
func New[M gmai.Model](factory func(...gmai.Option) M, opts ...gmai.Option) gmai.Model {
	return &model{Model: factory(protectedOptions(opts)...)}
}

type model struct{ gmai.Model }

func protectedOptions(opts []gmai.Option) []gmai.Option {
	o := gmai.NewOptions(opts...)
	if o.ToolHandler == nil {
		return opts
	}
	handler := o.ToolHandler
	return append(append([]gmai.Option(nil), opts...), gmai.WithToolHandler(func(ctx context.Context, call gmai.ToolCall) gmai.ToolResult {
		s := From(ctx)
		if s == nil {
			return gmai.ToolResult{ID: call.ID, Refused: "privacy", Content: `{"error":"Missing privacy context"}`}
		}
		if call.Input != nil {
			call.Input = s.Value(call.Input, true).(map[string]any)
		}
		result := handler(ctx, call)
		var value any
		// Prefer Content, the exact representation the normal provider sees.
		if decode(result.Content, &value) != nil {
			if result.Value != nil {
				if b, err := json.Marshal(result.Value); err == nil {
					_ = decode(string(b), &value)
				}
			}
			if value == nil {
				value = result.Content
			}
		}
		name := strings.ToLower(call.Name)
		if strings.Contains(name, "mail") {
			value = s.Mail(value)
		}
		if strings.Contains(name, "drive") {
			value = s.Drive(value)
		}
		if strings.Contains(name, "events") {
			value = s.Calendar(value)
		}
		value = s.Value(value, false)
		result.Value = value
		if str, ok := value.(string); ok {
			result.Content = str
		} else {
			b, _ := json.Marshal(value)
			result.Content = string(b)
		}
		return result
	}))
}

func (m *model) Init(opts ...gmai.Option) error { return m.Model.Init(protectedOptions(opts)...) }

func request(ctx context.Context, r *gmai.Request) (context.Context, *gmai.Request, *Session) {
	if From(ctx) == nil {
		ctx = With(ctx)
	}
	s := From(ctx)
	if r == nil {
		return ctx, nil, s
	}
	cp := *r
	cp.SystemPrompt = s.Value(r.SystemPrompt, false).(string) + "\nPrivacy placeholders beginning __MU_ represent private values. Preserve them exactly in answers and tool arguments; never guess their originals. [REDACTED] values are unavailable. For availability use events Free, not event details."
	cp.Prompt = s.Value(r.Prompt, false).(string)
	cp.Tools = append([]gmai.Tool(nil), r.Tools...)
	for i := range cp.Tools {
		cp.Tools[i].Description = s.Protect(cp.Tools[i].Description)
	}
	cp.Messages = make([]gmai.Message, len(r.Messages))
	for i, message := range r.Messages {
		cp.Messages[i] = gmai.Message{Role: message.Role, Content: s.Value(message.Content, false)}
	}
	return ctx, &cp, s
}

func response(s *Session, r *gmai.Response) *gmai.Response {
	if r == nil {
		return nil
	}
	cp := *r
	cp.Reply, cp.Answer = s.Value(r.Reply, true).(string), s.Value(r.Answer, true).(string)
	cp.ToolCalls = append([]gmai.ToolCall(nil), r.ToolCalls...)
	for i := range cp.ToolCalls {
		c := &cp.ToolCalls[i]
		if c.Input != nil {
			c.Input = s.Value(c.Input, true).(map[string]any)
		}
		c.Result, c.Error = s.Restore(c.Result), s.Restore(c.Error)
	}
	return &cp
}

func (m *model) Generate(ctx context.Context, r *gmai.Request, opts ...gmai.GenerateOption) (*gmai.Response, error) {
	ctx, r, s := request(ctx, r)
	resp, err := m.Model.Generate(ctx, r, opts...)
	return response(s, resp), err
}

func (m *model) Stream(ctx context.Context, r *gmai.Request, opts ...gmai.GenerateOption) (gmai.Stream, error) {
	ctx, r, s := request(ctx, r)
	stream, err := m.Model.Stream(ctx, r, opts...)
	if err != nil {
		return nil, err
	}
	return &streamReply{Stream: stream, session: s, text: &Text{session: s}}, nil
}

// Text restores placeholders even when token chunks split in their middle.
// A Text belongs to one ordered output stream, never to a shared account.
type Text struct {
	session *Session
	pending string
}

func (s *Session) Text() *Text { return &Text{session: s} }

func (t *Text) Write(chunk string) string {
	t.pending += chunk
	var ready strings.Builder
	for {
		start := strings.Index(t.pending, t.session.prefix)
		if start >= 0 {
			end := strings.Index(t.pending[start+len(t.session.prefix):], "__")
			if end < 0 {
				ready.WriteString(t.pending[:start])
				t.pending = t.pending[start:]
				break
			}
			end += start + len(t.session.prefix) + 2
			ready.WriteString(t.pending[:end])
			t.pending = t.pending[end:]
			continue
		}
		n := len(t.pending)
		for k := 1; k < len(t.session.prefix) && k <= len(t.pending); k++ {
			if strings.HasSuffix(t.pending, t.session.prefix[:k]) {
				n = len(t.pending) - k
			}
		}
		ready.WriteString(t.pending[:n])
		t.pending = t.pending[n:]
		break
	}
	return t.session.Restore(ready.String())
}

func (t *Text) Flush() string { out := t.session.Restore(t.pending); t.pending = ""; return out }

type streamReply struct {
	gmai.Stream
	session *Session
	text    *Text
	done    bool
}

func (s *streamReply) Recv() (*gmai.Response, error) {
	if s.done {
		return nil, io.EOF
	}
	r, err := s.Stream.Recv()
	if err == io.EOF {
		s.done = true
		if tail := s.text.Flush(); tail != "" {
			return &gmai.Response{Reply: tail}, nil
		}
	}
	if err != nil || r == nil {
		return r, err
	}
	cp := response(s.session, r)
	cp.Reply = s.text.Write(r.Reply)
	return cp, nil
}
