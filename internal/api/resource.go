package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

// ResourceID reconciles an identifier without changing the request method or
// treating typed content as a URL. The restored body remains available to the
// owning handler. Conflicting or repeated, differing identifiers are errors.
func ResourceID(w http.ResponseWriter, r *http.Request, pathID string) (*http.Request, bool) {
	id := ""
	add := func(value string) bool {
		if value == "" {
			return true
		}
		if id != "" && id != value {
			return false
		}
		id = value
		return true
	}
	conflict := func() (*http.Request, bool) {
		writeFailure(w, r, Fail(400, "invalid_arguments", "Conflicting resource IDs"))
		return r, false
	}
	if !add(pathID) {
		return conflict()
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeFailure(w, r, Fail(400, "invalid_arguments", "Invalid query parameters"))
		return r, false
	}
	for _, value := range values["id"] {
		if !add(value) {
			return conflict()
		}
	}
	media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch media {
	case "application/json":
		if r.Body != nil {
			data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
			if err != nil {
				writeFailure(w, r, Fail(400, "invalid_arguments", "Request body is too large or unreadable"))
				return r, false
			}
			r.Body = io.NopCloser(bytes.NewReader(data))
			if len(bytes.TrimSpace(data)) > 0 {
				// Token decoding also detects repeated id keys instead of taking the last.
				dec := json.NewDecoder(bytes.NewReader(data))
				tok, err := dec.Token()
				if err != nil || tok != json.Delim('{') {
					writeFailure(w, r, Fail(400, "invalid_arguments", "Send a JSON object"))
					return r, false
				}
				for dec.More() {
					key, err := dec.Token()
					if err != nil {
						writeFailure(w, r, Fail(400, "invalid_arguments", "Invalid JSON"))
						return r, false
					}
					var value json.RawMessage
					if err := dec.Decode(&value); err != nil {
						writeFailure(w, r, Fail(400, "invalid_arguments", "Invalid JSON"))
						return r, false
					}
					if key == "id" {
						var text string
						if len(value) == 0 || value[0] != '"' || json.Unmarshal(value, &text) != nil {
							writeFailure(w, r, Fail(400, "invalid_arguments", "id must be a string"))
							return r, false
						}
						if !add(text) {
							return conflict()
						}
					}
				}
				if _, err := dec.Token(); err != nil {
					writeFailure(w, r, Fail(400, "invalid_arguments", "Invalid JSON"))
					return r, false
				}
				if dec.Decode(new(any)) != io.EOF {
					writeFailure(w, r, Fail(400, "invalid_arguments", "Send one JSON object"))
					return r, false
				}
			}
		}
	case "application/x-www-form-urlencoded":
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
		if err := r.ParseForm(); err != nil {
			writeFailure(w, r, Fail(400, "invalid_arguments", "Invalid form"))
			return r, false
		}
		for _, value := range r.PostForm["id"] {
			if !add(value) {
				return conflict()
			}
		}
	}
	clone := r.Clone(r.Context())
	u := *r.URL
	clone.URL = &u
	if id != "" {
		values.Set("id", id)
		clone.URL.RawQuery = values.Encode()
		if r.Form != nil {
			clone.Form = cloneValues(r.Form)
			clone.Form.Set("id", id)
		}
		if r.PostForm != nil {
			clone.PostForm = cloneValues(r.PostForm)
			clone.PostForm.Set("id", id)
		}
	}
	return clone, true
}
func cloneValues(in url.Values) url.Values {
	out := url.Values{}
	for k, v := range in {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// ResourcePath accepts precisely one opaque path segment below a resource.
func ResourcePath(path, root string) (string, bool) {
	if path == root || path == root+"/" {
		return "", true
	}
	if !strings.HasPrefix(path, root+"/") {
		return "", false
	}
	id := strings.TrimPrefix(path, root+"/")
	return id, id != "" && !strings.ContainsAny(id, "/\\") && id != "." && id != ".."
}
