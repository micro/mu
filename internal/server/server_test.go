package server

// Tests that came with the code out of main.go. The write gate and the tool
// argument helpers live here now, so their tests do too.

import (
	"net/http/httptest"
	"testing"

	"mu/internal/quota"
)

func TestChargedWriteOp(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{name: "reads are free", method: "GET", path: "/social", want: ""},
		// /user/status went with statuses. A charge for a route that no longer
		// exists is a line nobody can reach and a price nobody can be quoted.
		{name: "a route that is gone", method: "POST", path: "/user/status", want: ""},
		{name: "social thread", method: "POST", path: "/social", want: quota.OpSocialPost},
		{name: "social reply", method: "POST", path: "/social/thread", want: quota.OpSocialReply},
		{name: "new blog post", method: "POST", path: "/blog", want: quota.OpBlogCreate},
		{name: "blog update free", method: "POST", path: "/blog?id=post-1", want: ""},
		{name: "blog comment", method: "POST", path: "/blog/post/post-1/comment", want: quota.OpBlogComment},
		{name: "app generation", method: "POST", path: "/apps/generate", want: quota.OpAppBuild},
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
