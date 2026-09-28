package inbox

import (
	"encoding/hex"
	"mu/internal/api"
	"mu/internal/thread"
	"net/http"
)

// Existing mailbox names keep their meaning. Conversation IDs are opaque
// 24-character hex IDs; resolve existing owned threads too for legacy IDs.
func inboxResource(w http.ResponseWriter, r *http.Request, owner string) (*http.Request, bool) {
	segment, valid := api.ResourcePath(r.URL.Path, "/inbox")
	id := ""
	if valid && segment != "" && len(inboxThreads(owner, r.URL.Path)) == 0 {
		_, hexErr := hex.DecodeString(segment)
		if thread.Get(owner, segment) != nil || (len(segment) == 24 && hexErr == nil) {
			id = segment
		}
	}
	normalized, ok := api.ResourceID(w, r, id)
	if !ok {
		return r, false
	}
	if id != "" {
		normalized.URL.Path = "/inbox"
	}
	return normalized, true
}
