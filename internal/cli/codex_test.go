package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCodexProbeOnlyInspectsAccountAndModels(t *testing.T) {
	replies := `{"id":1,"result":{}}
{"method":"notice","params":{}}
{"id":2,"result":{"account":{"type":"chatgpt","email":"private@example.com"}}}
{"id":3,"result":{"data":[{"model":"available-model","displayName":"Available"}],"nextCursor":"page2"}}
{"id":4,"result":{"data":[{"model":"second","displayName":"Second"}],"nextCursor":null}}
`
	var out bytes.Buffer
	status, err := probeCodex(strings.NewReader(replies), &out)
	if err != nil || status.AccountType != "chatgpt" || len(status.Models) != 2 {
		t.Fatalf("%+v %v", status, err)
	}
	methods := []string{"initialize", "initialized", "account/read", "model/list", "model/list"}
	dec := json.NewDecoder(&out)
	for _, want := range methods {
		var req map[string]any
		if err := dec.Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req["method"] != want || req["jsonrpc"] != nil {
			t.Fatalf("unexpected request: %v", req)
		}
	}
	if dec.More() {
		t.Fatal("extra request")
	}
}
func TestCodexProbeStopsWhenSignedOut(t *testing.T) {
	var out bytes.Buffer
	status, err := probeCodex(strings.NewReader("{\"id\":1,\"result\":{}}\n{\"id\":2,\"result\":{\"account\":null}}\n"), &out)
	if err != nil || status.AccountType != "" || strings.Contains(out.String(), "model/list") {
		t.Fatalf("%+v %v", status, err)
	}
}
