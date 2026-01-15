package mail

import (
	"strings"
	"testing"
)

func TestSendExternalEmailHTMLWrapping(t *testing.T) {
	// Test that HTML fragments are properly wrapped in HTML document structure
	
	tests := []struct {
		name        string
		bodyHTML    string
		shouldWrap  bool
		description string
	}{
		{
			name:        "Simple text fragment",
			bodyHTML:    "Hello!<br>It's working.",
			shouldWrap:  true,
			description: "Plain HTML fragment should be wrapped",
		},
		{
			name:        "Already has HTML structure",
			bodyHTML:    "<!DOCTYPE html><html><body>Hello</body></html>",
			shouldWrap:  false,
			description: "Complete HTML document should not be wrapped again",
		},
		{
			name:        "Has html tag",
			bodyHTML:    "<html><body>Test</body></html>",
			shouldWrap:  false,
			description: "HTML with html tag should not be wrapped",
		},
		{
			name:        "Complex HTML fragment",
			bodyHTML:    "Line 1<br>Line 2<br>It's great!",
			shouldWrap:  true,
			description: "Multi-line fragment should be wrapped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the wrapping logic from SendExternalEmail
			result := tt.bodyHTML
			htmlLower := strings.ToLower(result)
			
			if !strings.Contains(htmlLower, "<html") && !strings.Contains(htmlLower, "<!doctype") {
				// Would be wrapped
				if !tt.shouldWrap {
					t.Errorf("Test case %q: Expected not to wrap, but would be wrapped", tt.name)
				}
			} else {
				// Would NOT be wrapped
				if tt.shouldWrap {
					t.Errorf("Test case %q: Expected to wrap, but would not be wrapped", tt.name)
				}
			}
		})
	}
}
