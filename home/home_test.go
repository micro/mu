package home

import (
	"testing"
)

func TestHtmlEsc(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"<script>", "&lt;script&gt;"},
		{`"quoted"`, "&#34;quoted&#34;"},
		{"a & b", "a &amp; b"},
		{"", ""},
	}
	for _, tt := range tests {
		got := htmlEsc(tt.input)
		if got != tt.expected {
			t.Errorf("htmlEsc(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
