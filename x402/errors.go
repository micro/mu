package x402

import (
	"bytes"
	"mu/internal/app"
	"net/http"
	"strings"
)

// Only browser-page errors are held. Successful responses pass through; the
// protocol and webhook routes return before this wrapper is installed.
type browserResponse struct {
	http.ResponseWriter
	request *http.Request
	status  int
	body    bytes.Buffer
}

func (w *browserResponse) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	if status < 400 {
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *browserResponse) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if w.status >= 400 {
		return w.body.Write(p)
	}
	return w.ResponseWriter.Write(p)
}
func (w *browserResponse) finish() {
	if w.status < 400 {
		return
	}
	contentType := w.Header().Get("Content-Type")
	if contentType == "" || strings.HasPrefix(contentType, "text/plain") {
		w.Header().Del("Content-Length")
		app.Error(w.ResponseWriter, w.request, w.status, strings.TrimSpace(w.body.String()))
		return
	}
	w.ResponseWriter.WriteHeader(w.status)
	if w.request.Method != http.MethodHead {
		w.ResponseWriter.Write(w.body.Bytes())
	}
}
