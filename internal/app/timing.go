package app

import (
	"io"
	"mu/internal/auth"
	"mu/internal/usage"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/felixge/httpsnoop"
)

// TimeRequest preserves optional HTTP interfaces, including streaming and hijacking.
func TimeRequest(w http.ResponseWriter, r *http.Request) (http.ResponseWriter, func()) {
	start := time.Now()
	status, size := 200, int64(0)
	written := false
	wrapped := httpsnoop.Wrap(w, httpsnoop.Hooks{
		Flush: func(next httpsnoop.FlushFunc) httpsnoop.FlushFunc { return func() { written = true; next() } },
		ReadFrom: func(next httpsnoop.ReadFromFunc) httpsnoop.ReadFromFunc {
			return func(src io.Reader) (int64, error) { n, err := next(src); written = true; size += n; return n, err }
		},
		WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
			return func(code int) {
				if code >= 200 && !written {
					written = true
					status = code
				}
				next(code)
			}
		},
		Write: func(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
			return func(p []byte) (int, error) {
				n, err := next(p)
				written = true
				size += int64(n)
				return n, err
			}
		},
	})
	return wrapped, func() {
		if !usage.Skipped(r.URL.Path) && r.URL.Path != "/mu.css" && r.URL.Path != "/mu.js" && r.URL.Path != "/manifest.webmanifest" {
			usage.RecordOutcome(strconv.Itoa(status) + " " + usage.Endpoint(r.URL.Path))
		}

		if status >= 400 || (time.Since(start) > 10*time.Second && !strings.HasPrefix(wrapped.Header().Get("Content-Type"), "text/event-stream")) {
			account := ""
			if _, acc := auth.TrySession(r); acc != nil {
				account = acc.ID
			}
			outcome := usage.FailureKind(status, "")
			if status < 400 {
				outcome = "slow request"
			}
			usage.RecordActivity(usage.Activity{Surface: "http", Operation: r.Method + " " + usage.Endpoint(r.URL.Path), Account: account, Path: r.URL.Path, Host: r.Host, IP: ClientIP(r), UserAgent: r.UserAgent(), Status: status, Outcome: outcome, DurationMS: time.Since(start).Milliseconds()})
		}

		Log("http", "method=%s host=%q path=%q status=%d bytes=%d duration_ms=%.3f", r.Method, r.Host, r.URL.Path, status, size, float64(time.Since(start))/float64(time.Millisecond))
	}
}
