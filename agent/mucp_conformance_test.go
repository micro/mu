package agent

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/data"
)

// Exercise real HTTP authentication, Submit, the durable queue, result storage
// and ReadOperation with an independent client. Only model execution is replaced
// by a deterministic answer; no model bill or outside tool effect is needed in CI.
func TestMUCPIndependentDelegation(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("python3 required")
		}
		t.Skip("python3 unavailable")
	}
	replies.once.Do(func() {})
	replies.jobs = map[string]pendingReply{}
	replies.active = map[string]string{}
	replies.err = nil
	defer func() {
		replies.jobs = map[string]pendingReply{}
		if err := data.SaveJSON(repliesFile, replies.jobs); err != nil {
			t.Error(err)
		}
	}()
	const owner = "mucp_probe_agent"
	if err := auth.Create(&auth.Account{ID: owner, Admin: true, Created: time.Now()}); err != nil {
		t.Fatal(err)
	}
	defer auth.RemoveAccountForTest(owner)
	_, token, err := auth.CreateToken(owner, "agent-conformance", []string{"read", "write", "api:agent"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		MUCPHandler(w, r)
		if r.Header.Get("Micro-Endpoint") == "Assistant.Submit" {
			processReply(func(q AskRequest) (Answer, error) {
				calls.Add(1)
				recordAnswer(owner, q.On, "Acknowledged", "", "", q.AnswerRef)
				return Answer{Text: "Acknowledged"}, nil
			})
		}
	}))
	defer server.Close()
	receipt := filepath.Join(t.TempDir(), "receipt.json")
	for _, mode := range []string{"--delegate", "--resume"} {
		cmd := exec.Command(python, "../proto/conformance/probe.py", server.URL, mode, receipt, "--test-account", "--wait", "10")
		cmd.Env = append(os.Environ(), "MU_TOKEN="+token)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", mode, err, out)
		} else {
			t.Log(string(out))
		}
	}
	// SDKs use their own transports, not the independent conformance probe.
	if calls.Load() != 1 {
		t.Fatalf("probe executed %d times", calls.Load())
	}
	node, err := exec.LookPath("node")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("Node is required for SDK interoperability tests")
		}
		t.Skip("Node unavailable")
	}
	for _, command := range [][]string{
		{node, "../client/typescript/test/interop.mjs", server.URL},
		{python, "../client/python/tests/interop.py", server.URL},
	} {
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Env = append(os.Environ(), "MU_TOKEN="+token, "PYTHONPATH=../client/python")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("SDK: %v\n%s", err, out)
		} else {
			t.Log(string(out))
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("executed %d times after recovery and new-client resume", calls.Load())
	}
}
