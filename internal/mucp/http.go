// Package mucp implements Mu's HTTPS/JSON binding, independent of product handlers.
package mucp

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"sort"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"mu/internal/api"
	"mu/internal/auth"
)

const Version = "mucp/1"

var JSON = protojson.MarshalOptions{EmitUnpopulated: true}

type Operation struct {
	Service, Endpoint, Capability string
	Writes                        bool
	Request                       func() proto.Message
	Handle                        func(http.ResponseWriter, *http.Request, string, string, proto.Message) (proto.Message, error)
}

func Error(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Micro-Error", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": message, "status": status}})
}
func ValidID(id string) bool {
	return len(id) >= 16 && len(id) <= 128 && strings.IndexFunc(id, func(c rune) bool {
		return !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_')
	}) < 0
}

// Handler composes explicit operations. It never exposes the internal registry wholesale.
func Handler(operations []Operation) http.HandlerFunc {
	registry := map[string]Operation{}
	names := []string{}
	for _, op := range operations {
		key := op.Service + "/" + op.Endpoint
		if _, ok := registry[key]; ok {
			panic("duplicate MUCP operation: " + key)
		}
		registry[key] = op
		if op.Service == "assistant" {
			names = append(names, op.Endpoint)
		}
	}
	sort.Strings(names)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Micro-Protocol", Version)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			if v := r.Header.Values("Micro-Protocol"); len(v) > 0 && (len(v) != 1 || v[0] != Version) {
				Error(w, 400, "unsupported_protocol", "Unsupported MUCP version")
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"protocol": Version, "service": "assistant", "contract": "mu.assistant.v1", "encoding": "protobuf-json", "operations": names, "features": map[string]bool{"threadRecovery": true, "streaming": false, "cancellation": false, "automaticRetry": false, "durableSubmission": registry["assistant/Assistant.Submit"].Handle != nil}, "limits": map[string]int{"requestBytes": 65536, "waitSeconds": 120, "pageSize": 100}})
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "GET, POST")
			Error(w, 405, "method_not_allowed", "Use POST for calls")
			return
		}
		for _, h := range []string{"Micro-Protocol", "Micro-Service", "Micro-Endpoint", "Micro-Id"} {
			if len(r.Header.Values(h)) != 1 {
				Error(w, 400, "invalid_headers", "Supply one "+h+" header")
				return
			}
		}
		if r.Header.Get("Micro-Protocol") != Version {
			Error(w, 400, "unsupported_protocol", "Unsupported MUCP version")
			return
		}
		id := r.Header.Get("Micro-Id")
		if !ValidID(id) {
			Error(w, 400, "invalid_id", "Use a 16–128 character request ID")
			return
		}
		name, endpoint := r.Header.Get("Micro-Service"), r.Header.Get("Micro-Endpoint")
		op, ok := registry[name+"/"+endpoint]
		if !ok {
			Error(w, 404, "not_found", "Unknown operation")
			return
		}
		w.Header().Set("Micro-Id", id)
		w.Header().Set("Micro-Service", name)
		w.Header().Set("Micro-Endpoint", endpoint)
		kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || kind != "application/json" {
			Error(w, 415, "unsupported_encoding", "Use application/json")
			return
		}
		r = api.CredentialRequest(r)
		if op.Capability != "" {
			w.Header().Set("Micro-Error", "authorization_failed")
			if !api.AuthorizeProduct(w, r, op.Capability, op.Writes) {
				return
			}
			w.Header().Del("Micro-Error")
		}
		_, acc, err := auth.RequireSession(r)
		if err != nil || acc == nil {
			Error(w, 401, "unauthenticated", "Sign in to continue")
			return
		}
		if op.Capability == "" {
			if tok := auth.TokenFromRequest(r); tok != nil && (!tok.AllowsService(name) || (!op.Writes && !tok.HasPermission("read")) || (op.Writes && !tok.HasPermission("write"))) {
				Error(w, 403, "forbidden", "This token does not grant this service operation")
				return
			}
			if op.Writes && r.Header.Get("Authorization") == "" && r.Header.Get(api.TokenHeader) == "" && !auth.StrictCSRF(r) {
				Error(w, 403, "csrf", "A session write needs X-CSRF-Token")
				return
			}
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 65536))
		if err != nil {
			Error(w, 413, "too_large", "Request exceeds 64 KiB")
			return
		}
		request := op.Request()
		trim := strings.TrimSpace(string(raw))
		if len(trim) == 0 || trim[0] != '{' || protojson.Unmarshal(raw, request) != nil {
			Error(w, 400, "invalid_arguments", "Send one valid protobuf JSON object")
			return
		}
		if op.Writes {
			if err = auth.CheckPostRate(acc.ID); err != nil {
				Error(w, 429, "rate_limited", err.Error())
				return
			}
		}
		response, err := op.Handle(w, r, acc.ID, id, request)
		if err != nil {
			var e *api.Failure
			if errors.As(err, &e) {
				Error(w, e.Status, e.Code, e.Message)
			} else {
				Error(w, 500, "execution_failed", "The operation failed. Check its state before retrying.")
			}
			return
		}
		body, err := JSON.Marshal(response)
		if err != nil {
			Error(w, 500, "encoding_failed", "Response unavailable")
			return
		}
		_, _ = w.Write(body)
	}
}
