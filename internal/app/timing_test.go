package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestTimingPreservesStreamingAndResponse(t *testing.T) {
	underlying := httptest.NewRecorder()
	w, done := TimeRequest(underlying, httptest.NewRequest("POST", "/agent?secret=private", nil))
	flusher, ok := w.(http.Flusher)
	if !ok {
		t.Fatal("streaming interface lost")
	}
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("answer"))
	flusher.Flush()
	done()
	message := SysLog()[0].Message
	if !strings.Contains(message, "status=202 bytes=6") || strings.Contains(message, "private") {
		t.Fatalf("bad timing log: %s", message)
	}
	if underlying.Code != http.StatusAccepted || underlying.Body.String() != "answer" || !underlying.Flushed {
		t.Fatal("response changed")
	}
}
