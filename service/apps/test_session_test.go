package apps

import (
	"encoding/json"
	"net/http"
	"testing"

	"mu/internal/auth"
	"mu/internal/data"
)

// SDK probes are internal dispatches, not logins. A failed test must not leave
// a working credential behind, including when a handler panics or another
// login saves sessions while the probe is in flight.
func TestSDKProbeSessionLifetime(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const owner = "app_probe_owner"
	if err := auth.Create(&auth.Account{ID: owner, Secret: "test-only"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = auth.DeleteAccount(owner) })
	previous := http.DefaultServeMux
	t.Cleanup(func() { http.DefaultServeMux = previous })

	for _, outcome := range []string{"success", "http failure", "panic"} {
		t.Run(outcome, func(t *testing.T) {
			var probeToken string
			http.DefaultServeMux = http.NewServeMux()
			http.DefaultServeMux.HandleFunc("/news", func(w http.ResponseWriter, r *http.Request) {
				sess, err := auth.GetSession(r)
				if err != nil || sess.Account != owner {
					t.Fatalf("probe did not retain its caller identity: %v", err)
				}
				probeToken = sess.Token
				if sess.Type != "internal" {
					t.Errorf("probe minted a persistent login instead of an internal session")
				}
				// A normal login can happen before this handler returns. It must
				// not serialize the probe's temporary authority along with it.
				login, err := auth.CreateSession(owner)
				if err != nil {
					t.Fatal(err)
				}
				defer auth.Logout(login.Token)
				var saved map[string]*auth.Session
				if err := data.LoadJSON("sessions.json", &saved); err != nil {
					t.Fatal(err)
				}
				if _, ok := saved[sess.ID]; ok {
					t.Error("temporary probe credential reached persistent storage")
				}
				if _, ok := saved[login.ID]; !ok {
					t.Error("ordinary login was not persisted")
				}
				switch outcome {
				case "http failure":
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
				case "panic":
					panic("probe handler failed")
				default:
					_ = json.NewEncoder(w).Encode(map[string]any{"feed": []any{}})
				}
			})
			var panicked bool
			var result APITestResult
			func() {
				defer func() { panicked = recover() != nil }()
				result = executeSDKCall(sdkCall{call: "mu.news()", api: "news", path: "/news"}, owner)
			}()
			if panicked != (outcome == "panic") {
				t.Fatalf("unexpected panic state: %v", panicked)
			}
			if probeToken == "" {
				t.Fatal("probe was not dispatched")
			}
			if _, err := auth.ParseToken(probeToken); err == nil {
				t.Error("probe credential still works after dispatch ended")
			}
			if outcome == "success" && (result.Status != http.StatusOK || result.Error != "") {
				t.Errorf("successful probe changed: %+v", result)
			}
			if outcome == "http failure" && (result.Status != http.StatusServiceUnavailable || result.Error == "") {
				t.Errorf("failed probe was not reported: %+v", result)
			}
		})
	}
}

func TestSDKProbeRejectsMissingCaller(t *testing.T) {
	previous := http.DefaultServeMux
	t.Cleanup(func() { http.DefaultServeMux = previous })
	http.DefaultServeMux = http.NewServeMux()
	http.DefaultServeMux.HandleFunc("/news", func(http.ResponseWriter, *http.Request) {
		t.Fatal("probe dispatched without a valid caller")
	})
	result := executeSDKCall(sdkCall{call: "mu.news()", api: "news", path: "/news"}, "missing-probe-owner")
	if result.Error != "auth failed" {
		t.Fatalf("invalid identity did not fail closed: %+v", result)
	}
}
