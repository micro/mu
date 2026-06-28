package app

import "testing"

func TestParseTokenExpiresIn(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{name: "empty", in: "", want: 0},
		{name: "never", in: "0", want: 0},
		{name: "negative", in: "-1", want: 0},
		{name: "numeric days", in: "90", want: 90},
		{name: "numeric days trimmed", in: " 365 ", want: 365},
		{name: "legacy date", in: "2026-12-31", want: 365},
		{name: "invalid", in: "soon", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseTokenExpiresIn(tt.in); got != tt.want {
				t.Fatalf("parseTokenExpiresIn(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseTokenPermissions(t *testing.T) {
	got := parseTokenPermissions(" read, write, ,admin ")
	want := []string{"read", "write", "admin"}
	if len(got) != len(want) {
		t.Fatalf("parseTokenPermissions returned %d values, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseTokenPermissions()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
