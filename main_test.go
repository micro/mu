package main

import (
	"net/http/httptest"
	"testing"

	"mu/wallet"
)

func TestIsServerMode(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "no args", args: nil, want: false},
		{name: "cli command", args: []string{"news"}, want: false},
		{name: "long flag", args: []string{"--serve"}, want: true},
		{name: "short flag", args: []string{"-serve"}, want: true},
		{name: "long flag with value", args: []string{"--serve=false"}, want: true},
		{name: "short flag with value", args: []string{"-serve=true"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isServerMode(tt.args); got != tt.want {
				t.Fatalf("isServerMode(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestChargedWriteOp(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{name: "reads are free", method: "GET", path: "/social", want: ""},
		{name: "status post", method: "POST", path: "/user/status", want: wallet.OpSocialPost},
		{name: "social thread", method: "POST", path: "/social", want: wallet.OpSocialPost},
		{name: "social reply", method: "POST", path: "/social/thread", want: wallet.OpSocialReply},
		{name: "new blog post", method: "POST", path: "/blog", want: wallet.OpBlogCreate},
		{name: "blog update free", method: "POST", path: "/blog?id=post-1", want: ""},
		{name: "blog comment", method: "POST", path: "/blog/post/post-1/comment", want: wallet.OpBlogComment},
		{name: "app generation", method: "POST", path: "/apps/build/generate", want: wallet.OpAppBuild},
		{name: "uncharged post", method: "POST", path: "/mail", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.path, nil)
			if got := chargedWriteOp(r); got != tt.want {
				t.Fatalf("chargedWriteOp(%s %s) = %q, want %q", tt.method, tt.path, got, tt.want)
			}
		})
	}
}

func TestRecallFormattingHelpers(t *testing.T) {
	if got := recallStripTags("<p>Hello <strong>world</strong></p>"); got != "Hello world" {
		t.Fatalf("recallStripTags() = %q", got)
	}
	if got := recallSnippet("<p>Hello\n\tworld</p>", 20); got != "Hello world" {
		t.Fatalf("recallSnippet() = %q", got)
	}
	if got := recallSnippet("abcdef", 3); got != "abc…" {
		t.Fatalf("truncated recallSnippet() = %q", got)
	}
	if got := recallFirstLine(" first line \n second line", 20); got != "first line" {
		t.Fatalf("recallFirstLine() = %q", got)
	}
	if got := recallFirstLine("abcdef", 3); got != "abc…" {
		t.Fatalf("truncated recallFirstLine() = %q", got)
	}
}

func TestArgFloat(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want float64
	}{
		{name: "float", in: 1.25, want: 1.25},
		{name: "int", in: 2, want: 2},
		{name: "string", in: "3.5", want: 3.5},
		{name: "invalid string", in: "nope", want: 0},
		{name: "unsupported", in: true, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := argFloat(tt.in); got != tt.want {
				t.Fatalf("argFloat(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
