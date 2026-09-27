package mail

// JMAP is another authenticated view of Mail, sharing IMAP's folder projection.
// Unsupported mutations return explicit errors rather than silently losing mail.
import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/internal/auth"
	"mu/internal/origin"
)

const jmapCore = "urn:ietf:params:jmap:core"
const jmapMail = "urn:ietf:params:jmap:mail"

type jmapObject = map[string]any

func jmapError(kind string) jmapObject { return jmapObject{"type": kind} }
func jmapJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func jmapID(s string) string     { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
func jmapDecode(s string) string { b, _ := base64.RawURLEncoding.DecodeString(s); return string(b) }

var jmapRequests struct {
	sync.Mutex
	active map[string]int
	total  int
}

func jmapAcquire(owner string) bool {
	jmapRequests.Lock()
	defer jmapRequests.Unlock()
	if jmapRequests.active == nil {
		jmapRequests.active = map[string]int{}
	}
	if jmapRequests.total >= 32 || jmapRequests.active[owner] >= 4 {
		return false
	}
	jmapRequests.active[owner]++
	jmapRequests.total++
	return true
}
func jmapRelease(owner string) {
	jmapRequests.Lock()
	defer jmapRequests.Unlock()
	jmapRequests.active[owner]--
	jmapRequests.total--
	if jmapRequests.active[owner] == 0 {
		delete(jmapRequests.active, owner)
	}
}

// JMAPHandler never accepts cookies: use the same scoped mail token as IMAP.
func JMAPHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	user, token, ok := r.BasicAuth()
	if !ok && strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		user, _ = auth.ValidatePAT(token)
	}
	acc, err := accountForToken(user, token)
	if err != nil || acc == nil || acc.Banned {
		w.Header().Set("WWW-Authenticate", `Basic realm="Micro mail", charset="UTF-8"`)
		http.Error(w, "Mail token required", 401)
		return
	}
	tokenID, err := auth.ValidatePATToken(token)
	grant, grantErr := auth.TokenByID(tokenID)
	if err != nil || grantErr != nil || !grant.HasPermission("read") {
		http.Error(w, "Mail read permission required", 403)
		return
	}
	mayWrite := grant.HasPermission("write")
	if !jmapAcquire(acc.ID) {
		http.Error(w, "Too many simultaneous mail requests", 429)
		return
	}
	defer jmapRelease(acc.ID)
	switch {
	case r.URL.Path == "/.well-known/jmap" || r.URL.Path == "/mail/jmap/session":
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		base := strings.TrimRight(origin.URL(r), "/")
		jmapJSON(w, jmapObject{"capabilities": jmapObject{jmapCore: jmapObject{"maxSizeUpload": 0, "maxConcurrentUpload": 1, "maxSizeRequest": 1048576, "maxConcurrentRequests": 4, "maxCallsInRequest": 32, "maxObjectsInGet": 500, "maxObjectsInSet": 100, "collationAlgorithms": []string{"i;unicode-casemap"}}, jmapMail: jmapObject{}}, "accounts": jmapObject{acc.ID: jmapObject{"name": acc.ID, "isPersonal": true, "isReadOnly": !mayWrite, "accountCapabilities": jmapObject{jmapMail: jmapObject{"maxMailboxesPerEmail": nil, "maxMailboxDepth": nil, "maxSizeMailboxName": 255, "maxSizeAttachmentsPerEmail": 0, "emailQuerySortOptions": []string{"receivedAt", "subject"}, "mayCreateTopLevelMailbox": false}}}}, "primaryAccounts": jmapObject{jmapMail: acc.ID}, "username": acc.ID, "apiUrl": base + "/mail/jmap", "downloadUrl": base + "/mail/jmap/download/{accountId}/{blobId}/{name}?type={type}", "uploadUrl": base + "/mail/jmap/upload/{accountId}", "eventSourceUrl": base + "/mail/jmap/events?types={types}&closeafter={closeafter}&ping={ping}", "state": "1"})
	case r.URL.Path == "/mail/jmap":
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "Use application/json", 415)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var req struct {
			Using []string            `json:"using"`
			Calls [][]json.RawMessage `json:"methodCalls"`
		}
		dec := json.NewDecoder(r.Body)
		if dec.Decode(&req) != nil || dec.Decode(new(any)) != io.EOF || len(req.Calls) > 32 {
			http.Error(w, "Invalid JMAP request", 400)
			return
		}
		using := map[string]bool{}
		for _, cap := range req.Using {
			if cap != jmapCore && cap != jmapMail {
				w.WriteHeader(400)
				jmapJSON(w, jmapObject{"type": "urn:ietf:params:jmap:error:unknownCapability"})
				return
			}
			using[cap] = true
		}
		if !using[jmapCore] {
			http.Error(w, "Core capability required", 400)
			return
		}
		// Validate the entire envelope before executing any mutations.
		for _, call := range req.Calls {
			if len(call) != 3 {
				http.Error(w, "Invalid method call", 400)
				return
			}
			var name, id string
			var args jmapObject
			if json.Unmarshal(call[0], &name) != nil || json.Unmarshal(call[2], &id) != nil || json.Unmarshal(call[1], &args) != nil || args == nil {
				http.Error(w, "Invalid method call", 400)
				return
			}
		}
		responses := [][3]any{}
		for _, call := range req.Calls {
			if len(call) != 3 {
				http.Error(w, "Invalid method call", 400)
				return
			}
			var name, id string
			var args jmapObject
			if json.Unmarshal(call[0], &name) != nil || json.Unmarshal(call[2], &id) != nil || json.Unmarshal(call[1], &args) != nil || args == nil {
				http.Error(w, "Invalid method call", 400)
				return
			}
			for key, value := range args {
				if strings.HasPrefix(key, "#") {
					resolved, ok := jmapReference(value, responses)
					if !ok {
						args = nil
						break
					}
					delete(args, key)
					args[strings.TrimPrefix(key, "#")] = resolved
				}
			}
			responseName := name
			var result jmapObject
			switch {
			case args == nil:
				result = jmapError("invalidResultReference")
			case name == "Core/echo":
				result = args
			case !using[jmapMail]:
				result = jmapError("unknownMethod")
			case strings.HasSuffix(name, "/set") && !mayWrite:
				result = jmapError("forbidden")
			case args["accountId"] != acc.ID:
				result = jmapError("accountNotFound")
			default:
				result = jmapMethod(acc.ID, name, args)
			}
			if result["type"] != nil {
				responseName = "error"
			}
			responses = append(responses, [3]any{responseName, result, id})
		}
		jmapJSON(w, jmapObject{"methodResponses": responses, "sessionState": "1"})
	case strings.HasPrefix(r.URL.Path, "/mail/jmap/download/"):
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/mail/jmap/download/"), "/")
		if len(parts) < 3 || parts[0] != acc.ID {
			http.NotFound(w, r)
			return
		}
		snap := jmapSnapshot(acc.ID)
		blob := jmapDecode(parts[1])
		attachment := strings.HasPrefix(blob, "attachment:")
		id := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(blob, "attachment:"), "message:"), "body:")
		for _, m := range snap.messages {
			if m.ID == id {
				var body []byte
				if attachment {
					body, err = base64.StdEncoding.DecodeString(m.Attachment)
					if err != nil || m.Attachment == "" {
						break
					}
					w.Header().Set("Content-Type", "application/octet-stream")
				} else if strings.HasPrefix(blob, "body:") {
					body = []byte(m.Body)
					if m.Markdown || imapLooksHTML(m.Body) {
						body = []byte(Rendered(m))
					}
					w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				} else if strings.HasPrefix(blob, "message:") {
					body = imapRender(m)
					w.Header().Set("Content-Type", "message/rfc822")
				} else {
					break
				}
				w.Header().Set("Content-Disposition", `attachment; filename="message"`)
				w.Header().Set("X-Content-Type-Options", "nosniff")
				_, _ = w.Write(body)
				return
			}
		}
		http.NotFound(w, r)
	case r.URL.Path == "/mail/jmap/events":
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		jmapEvents(w, r, acc.ID, user, token)
	default:
		http.Error(w, "JMAP operation not supported", 405)
	}
}

func jmapReference(value any, responses [][3]any) (any, bool) {
	ref, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	for _, r := range responses {
		if r[2] != ref["resultOf"] || r[0] != ref["name"] {
			continue
		}
		path, ok := ref["path"].(string)
		if !ok {
			return nil, false
		}
		return jmapPointer(r[1], strings.Split(strings.TrimPrefix(path, "/"), "/"))
	}
	return nil, false
}
func jmapPointer(value any, path []string) (any, bool) {
	if len(path) == 0 {
		return value, true
	}
	if path[0] == "*" {
		items, ok := value.([]any)
		if !ok {
			return nil, false
		}
		out := []any{}
		for _, item := range items {
			v, ok := jmapPointer(item, path[1:])
			if !ok {
				return nil, false
			}
			if list, ok := v.([]any); ok {
				out = append(out, list...)
			} else {
				out = append(out, v)
			}
		}
		return out, true
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	v, ok := obj[path[0]]
	if !ok {
		return nil, false
	}
	return jmapPointer(v, path[1:])
}
func jmapHash(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func jmapEvents(w http.ResponseWriter, r *http.Request, owner, user, token string) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	deadline := time.NewTimer(2 * time.Minute)
	defer deadline.Stop()
	last := ""
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		acc, err := accountForToken(user, token)
		if err != nil || acc.Banned {
			return
		}
		state := jmapSnapshot(owner).state
		if state != last {
			payload, _ := json.Marshal(jmapObject{"@type": "StateChange", "changed": jmapObject{owner: jmapObject{"Email": state, "Mailbox": state, "Thread": state}}})
			fmt.Fprintf(w, "event: state\ndata: %s\n\n", payload)
			last = state
		} else {
			fmt.Fprint(w, ": keepalive\n\n")
		}
		f.Flush()
		if r.URL.Query().Get("closeafter") == "state" {
			return
		}
		select {
		case <-deadline.C:
			return
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
