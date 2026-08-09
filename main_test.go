package main

// What is left in package main is the dispatch between the two programs, so
// that is what is tested here. The middleware and tool-argument helpers moved
// to internal/server with the code they cover.

import "testing"

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
