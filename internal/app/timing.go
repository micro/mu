package app

import (
	"io"
	"net/http"
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
		Log("http", "method=%s path=%q status=%d bytes=%d duration_ms=%.3f", r.Method, r.URL.Path, status, size, float64(time.Since(start))/float64(time.Millisecond))
	}
}
