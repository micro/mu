package editorial

import (
	"strings"
	"testing"
)

func TestRejectsObservedArtifacts(t *testing.T) {
	body := strings.Repeat("A sourced article sentence with useful information. ", 8) + "[Source](https://example.com/story)"
	for _, text := range []string{`<?xml version="1.0" encoding="UTF-8"?>`, "<function_calls>", "<invoke name=\"news_read\">", "Let's read the article.</think>I'll read it.</think>", "<THINK>reasoning", "</analysis>"} {
		if Validate(text, body) == nil || Validate("A valid editorial title", text+body) == nil {
			t.Fatalf("accepted %q", text)
		}
	}
	if err := Validate("A valid editorial title", body); err != nil {
		t.Fatal(err)
	}
	if Validate("A valid editorial title", "2c12c6902932863f") == nil {
		t.Fatal("accepted identifier as body")
	}
}
