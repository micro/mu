package web

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mu/internal/auth"
	"net/http"
	"net/url"
	"strings"
)

type dataHandlerKey struct{}

// WithData lets HTML responses prepare their initial view data through the same
// authenticated handlers used for later refreshes. No external HTTP round trip.
func WithData(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), dataHandlerKey{}, next)))
	})
}

type viewResponse struct {
	header   http.Header
	body     bytes.Buffer
	status   int
	overflow bool
}

func (v *viewResponse) Header() http.Header  { return v.header }
func (v *viewResponse) WriteHeader(code int) { v.status = code }
func (v *viewResponse) Write(p []byte) (int, error) {
	if v.overflow || v.body.Len()+len(p) > 4<<20 {
		v.overflow = true
		return len(p), nil
	}
	return v.body.Write(p)
}
func initialData(r *http.Request) map[string]any {
	next, _ := r.Context().Value(dataHandlerKey{}).(http.Handler)
	if next == nil || r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/agent/") && r.URL.Path != "/agent/new" {
		return nil
	}
	read := func(path, body string) any {
		target, err := url.Parse(path)
		if err != nil {
			return nil
		}
		req := r.Clone(r.Context())
		req.URL = target
		req.RequestURI = target.RequestURI()
		req.Method = http.MethodGet
		req.Form = nil
		req.PostForm = nil
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		req.Body = nil
		if body != "" {
			req.Method = http.MethodPost
			req.Body = io.NopCloser(strings.NewReader(body))
			req.ContentLength = int64(len(body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if body == "{}" {
				req.Header.Set("Content-Type", "application/json")
			}
		}
		out := &viewResponse{header: make(http.Header), status: 200}
		next.ServeHTTP(out, req)
		if out.overflow || out.status != 200 || !strings.Contains(out.header.Get("Content-Type"), "application/json") {
			return nil
		}
		var data any
		if json.Unmarshal(out.body.Bytes(), &data) != nil {
			return nil
		}
		return data
	}
	path, body := r.URL.RequestURI(), ""
	data := map[string]any{}
	if r.URL.Path == "/events" {
		data["brief"] = read("/events?view=brief", "")
	}
	if r.URL.Path == "/files" {
		data["ssh"] = read("/client/ssh", "")
	}
	switch {
	case r.URL.Path == "/about" || r.URL.Path == "/privacy":
		return nil
	case r.URL.Path == "/agents" || r.URL.Path == "/agent/new":
		path = "/client/agents"
		data["services"] = read("/client/services", "")
	case strings.HasPrefix(r.URL.Path, "/services") || strings.HasPrefix(r.URL.Path, "/service/"):
		path = "/client/services"
	case r.URL.Path == "/blog/post" || r.URL.Path == "/social/thread":
		q := r.URL.Query()
		q.Set("client", "1")
		path = r.URL.Path + "?" + q.Encode()
	case r.URL.Path == "/work":
		path = "/tasks"
		data["apps"] = read("/apps", "")
	case strings.HasPrefix(r.URL.Path, "/admin/") && r.URL.Path != "/admin/users":
		path = "/admin/client?page=" + url.QueryEscape(strings.TrimPrefix(r.URL.Path, "/admin/"))
	case r.URL.Path == "/docs":
		path = "/docs"
		body = "action=search&q="
	case r.URL.Path == "/notes":
		path = "/notes"
	case r.URL.Path == "/contacts":
		path = "/client/call/contacts/list"
		body = "{}"
	case r.URL.Path == "/files":
		path = "/client/call/files/list"
		body = "{}"
	case r.URL.Path == "/mail":
		q := r.URL.Query()
		q.Set("client", "1")
		path = "/mail?" + q.Encode()
	case r.URL.Path == "/bookmarks" && r.URL.Query().Get("id") == "":
		path = "/bookmarks/search"
		body = "query=&offset=0"
	case r.URL.Path == "/recall":
		path = "/recall"
		body = "q="
	case r.URL.Path == "/apps/new":
		data["page"] = map[string]any{"name": "", "description": "", "html": "", "tags": "", "public": false, "price": 0}
		return data
	case strings.HasPrefix(r.URL.Path, "/apps/") && strings.HasSuffix(r.URL.Path, "/edit"):
		path = strings.TrimSuffix(r.URL.Path, "/edit")
	}
	value := read(path, body)
	if strings.HasPrefix(path, "/client/call/") {
		if v, ok := value.(map[string]any); ok {
			value = v["data"]
		}
	}
	if r.URL.Path == "/images" {
		if v, ok := value.(map[string]any); ok {
			items := []any{}
			if v["daily"] != nil {
				items = append(items, v["daily"])
			}
			if stock, ok := v["stock"].([]any); ok {
				items = append(items, stock...)
			}
			value = map[string]any{"items": items}
		}
	}
	if r.URL.Path == "/bookmarks" && r.URL.Query().Get("id") != "" && value != nil {
		value = map[string]any{"items": []any{value}, "total": 1}
	}
	if value != nil {
		data["page"] = value
	}
	return data
}
