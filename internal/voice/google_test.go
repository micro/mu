package voice

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestGoogleAudioBoundaries(t *testing.T) {
	t.Setenv("GOOGLE_SPEECH_API_KEY", "private-key")
	old := client
	t.Cleanup(func() { client = old })
	requests := 0
	client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.RawQuery != "" || r.Header.Get("X-Goog-Api-Key") != "private-key" {
			t.Fatal("credential not confined to header")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		out := `{"results":[{"alternatives":[{"transcript":"Hello Micro."}]}]}`
		if strings.Contains(r.URL.Host, "texttospeech") {
			input := body["input"].(map[string]any)
			if input["text"] != "Hello" || input["ssml"] != nil {
				t.Fatal("not plain text")
			}
			out = `{"audioContent":"` + base64.StdEncoding.EncodeToString([]byte("mp3")) + `"}`
		} else if body["config"].(map[string]any)["encoding"] != "LINEAR16" {
			t.Fatal("incorrect encoding")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(out))}, nil
	})}
	text, err := Transcribe(context.Background(), []byte{0, 0}, 16000, "en-GB")
	if err != nil || text != "Hello Micro." {
		t.Fatalf("%q %v", text, err)
	}
	audio, err := Speak(context.Background(), "Hello")
	if err != nil || string(audio) != "mp3" {
		t.Fatalf("%q %v", audio, err)
	}
	for _, audio := range [][]byte{nil, {0}, make([]byte, 16000*2*30+2)} {
		if _, err := Transcribe(context.Background(), audio, 16000, "en-GB"); err == nil {
			t.Fatal("invalid audio accepted")
		}
	}
	if _, err := Speak(context.Background(), strings.Repeat("x", 4501)); err == nil {
		t.Fatal("unbounded speech")
	}
	if requests != 2 {
		t.Fatalf("invalid requests reached provider: %d", requests)
	}
	client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader("private upstream details"))}, nil
	})}
	if _, err := Speak(context.Background(), "Hello"); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("upstream error leaked")
	}
}
func TestCallCodesAreScopedExpiringAndSingleUse(t *testing.T) {
	code, err := NewCode("alice")
	if err != nil {
		t.Fatal(err)
	}
	if ConsumeCode("bob", code) {
		t.Fatal("cross-account code")
	}
	if !ConsumeCode("alice", code) || ConsumeCode("alice", code) {
		t.Fatal("code not single-use")
	}
	code, _ = NewCode("alice")
	codes.Lock()
	c := codes.values["alice"]
	c.until = time.Now().Add(-time.Second)
	codes.values["alice"] = c
	codes.Unlock()
	if ConsumeCode("alice", code) {
		t.Fatal("expired code")
	}
	code, _ = NewCode("alice")
	for i := 0; i < 5; i++ {
		ConsumeCode("alice", "bad")
	}
	if ConsumeCode("alice", code) {
		t.Fatal("guess limit bypassed")
	}
}
