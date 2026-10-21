package blog

import (
	"errors"
	"mu/internal/ai"
	"strings"
	"testing"
)

func TestPublicationReviewFailsClosed(t *testing.T) {
	body := strings.Repeat("A useful sourced statement about the day's news. ", 8) + "[Source](https://example.com/story)"
	for _, tc := range []struct {
		answer string
		err    error
		ok     bool
	}{
		{`{"approved":true,"reason":"Supported by sources"}`, nil, true},
		{`{"approved":false,"reason":"Unsupported claim"}`, nil, false},
		{`{"approved":true}`, nil, false},
		{`{"reason":"fine"}`, nil, false},
		{"APPROVED", nil, false},
		{"", errors.New("timeout"), false},
		{"<think>fine</think>" + `{"approved":true,"reason":"fine"}`, nil, false},
	} {
		err := review("A valid article title", body, "Source evidence", func(p *ai.Prompt) (string, error) {
			if p.Caller != "publication-review" || !strings.Contains(p.Question, "Source evidence") {
				t.Fatal("missing source review")
			}
			return tc.answer, tc.err
		})
		if (err == nil) != tc.ok {
			t.Fatalf("%s: %v", tc.answer, err)
		}
	}
	calls := 0
	err := review("<?xml version=\"1.0\"?>", body, "sources", func(*ai.Prompt) (string, error) { calls++; return "", nil })
	if err == nil || calls != 0 {
		t.Fatal("malformed draft reached reviewer")
	}
}
