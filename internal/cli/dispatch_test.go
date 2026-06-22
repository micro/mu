package cli

import (
	"reflect"
	"testing"
)

func TestParseToolFlags(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantArgs map[string]any
		wantPos  []string
	}{
		{
			name:     "no args",
			args:     nil,
			wantArgs: map[string]any{},
			wantPos:  nil,
		},
		{
			name: "space-separated string",
			args: []string{"--query", "ai safety"},
			wantArgs: map[string]any{
				"query": "ai safety",
			},
		},
		{
			name: "equals-separated string",
			args: []string{"--query=ai safety"},
			wantArgs: map[string]any{
				"query": "ai safety",
			},
		},
		{
			name: "integer coerced",
			args: []string{"--limit", "10"},
			wantArgs: map[string]any{
				"limit": int64(10),
			},
		},
		{
			name: "float coerced",
			args: []string{"--lat", "51.5", "--lon", "-0.12"},
			wantArgs: map[string]any{
				"lat": 51.5,
				"lon": -0.12,
			},
		},
		{
			name: "bool explicit",
			args: []string{"--pollen", "true"},
			wantArgs: map[string]any{
				"pollen": true,
			},
		},
		{
			name: "bare bool flag",
			args: []string{"--public"},
			wantArgs: map[string]any{
				"public": true,
			},
		},
		{
			name: "positional trailing",
			args: []string{"--query", "ai", "extra"},
			wantArgs: map[string]any{
				"query": "ai",
			},
			wantPos: []string{"extra"},
		},
		{
			name:     "only positional",
			args:     []string{"hello world"},
			wantArgs: map[string]any{},
			wantPos:  []string{"hello world"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, pos, err := parseToolFlags(tc.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.wantArgs) {
				t.Errorf("args mismatch:\n want %v\n  got %v", tc.wantArgs, got)
			}
			if !reflect.DeepEqual(pos, tc.wantPos) {
				t.Errorf("positional mismatch:\n want %v\n  got %v", tc.wantPos, pos)
			}
		})
	}
}

func TestCoerce(t *testing.T) {
	cases := []struct {
		in   string
		want any
	}{
		{"hello", "hello"},
		{"true", true},
		{"false", false},
		{"10", int64(10)},
		{"-5", int64(-5)},
		{"1.5", 1.5},
		{"51.5", 51.5},
	}
	for _, tc := range cases {
		got := coerce(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("coerce(%q) = %v (%T), want %v (%T)", tc.in, got, got, tc.want, tc.want)
		}
	}
}

func TestDefaultArgKey(t *testing.T) {
	cases := map[string]string{
		"chat":          "prompt",
		"agent":         "prompt",
		"apps_build":    "prompt",
		"news_search":   "query",
		"video_search":  "query",
		"web_search":    "q",
		"places_search": "q",
		"web_fetch":     "url",
		"blog_read":     "id",
	}
	for tool, want := range cases {
		got, ok := defaultArgKey(tool)
		if !ok {
			t.Errorf("defaultArgKey(%q) returned false, want %q", tool, want)
			continue
		}
		if got != want {
			t.Errorf("defaultArgKey(%q) = %q, want %q", tool, got, want)
		}
	}
	if _, ok := defaultArgKey("mail_send"); ok {
		t.Error("defaultArgKey(mail_send) should return false")
	}
}
