package exercise

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			w.Write([]byte(`{"healthy":true}`))
		case "/boom":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	ok200 := func(s int, _ []byte) error {
		if s != http.StatusOK {
			return errStatus(s)
		}
		return nil
	}
	probes := []Probe{
		{Name: "status", Method: "GET", Path: "/status?format=json", Check: func(s int, b []byte) error {
			if s != 200 || string(b) == "" {
				return errStatus(s)
			}
			return nil
		}},
		{Name: "landing", Method: "GET", Path: "/", Check: ok200},
		{Name: "boom", Method: "GET", Path: "/boom", Check: ok200},
	}

	rep := Run(context.Background(), srv.URL, probes, 3, srv.Client())

	if rep.Runs != 3 {
		t.Errorf("runs = %d, want 3", rep.Runs)
	}
	if rep.Passed != 2 || rep.Failed != 1 {
		t.Errorf("passed/failed = %d/%d, want 2/1", rep.Passed, rep.Failed)
	}
	if rep.Healthy() {
		t.Error("report should be unhealthy (boom failed)")
	}
	// Find the boom result.
	var boom *Result
	for i := range rep.Results {
		if rep.Results[i].Name == "boom" {
			boom = &rep.Results[i]
		}
	}
	if boom == nil || boom.Fail != 3 || boom.OK != 0 {
		t.Fatalf("boom result wrong: %+v", boom)
	}
	if boom.LastError == "" {
		t.Error("boom should record an error")
	}
}

func TestBattery(t *testing.T) {
	if len(Battery(false)) == 0 {
		t.Fatal("empty battery")
	}
	if len(Battery(true)) <= len(Battery(false)) {
		t.Error("deep battery should add the agent probe")
	}
}

func errStatus(s int) error { return &statusErr{s} }

type statusErr struct{ code int }

func (e *statusErr) Error() string { return http.StatusText(e.code) }
