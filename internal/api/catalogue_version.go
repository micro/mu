package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
)

// catalogueHeaders identifies the discoverable schema without changing the
// catalogue response shape. Clients can compare it after reconnecting.
func catalogueHeaders(w http.ResponseWriter, r *http.Request) {
	type entry struct {
		Name, Description string
		Params            []ToolParam
	}
	var entries []entry
	for _, t := range mcpToolsFor(r) {
		if t.OperatorOnly && !operatorAllowed(r) {
			continue
		}
		entries = append(entries, entry{t.Name, t.Description, t.Params})
	}
	raw, _ := json.Marshal(entries)
	sum := sha256.Sum256(raw)
	w.Header().Set("X-Mu-Catalogue-Version", hex.EncodeToString(sum[:8]))
	w.Header().Set("X-Mu-Tool-Count", strconv.Itoa(len(entries)))
}
