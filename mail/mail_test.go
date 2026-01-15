package mail

import (
	"testing"
)

func TestConvertPlainTextToHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple text with apostrophe",
			input:    "Hello! It's a great day.",
			expected: "Hello! It's a great day.",
		},
		{
			name:     "Text with newlines",
			input:    "Line 1\nLine 2\nLine 3",
			expected: "Line 1<br>Line 2<br>Line 3",
		},
		{
			name:     "Text with HTML characters",
			input:    "5 < 10 and 10 > 5",
			expected: "5 &lt; 10 and 10 &gt; 5",
		},
		{
			name:     "Text with ampersand",
			input:    "Tom & Jerry",
			expected: "Tom &amp; Jerry",
		},
		{
			name:     "Text with quotes",
			input:    `He said "hello" to me`,
			expected: `He said "hello" to me`,
		},
		{
			name:     "Complex text with multiple special chars",
			input:    "Hi there!\nIt's <important> to escape & properly.\nDon't you think?",
			expected: "Hi there!<br>It's &lt;important&gt; to escape &amp; properly.<br>Don't you think?",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Text with only newlines",
			input:    "\n\n\n",
			expected: "<br><br><br>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertPlainTextToHTML(tt.input)
			if result != tt.expected {
				t.Errorf("convertPlainTextToHTML() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestConvertPlainTextToHTMLPreservesApostrophesAndQuotes(t *testing.T) {
	// Specific test to verify apostrophes and quotes are NOT escaped
	input := "It's important that we don't escape apostrophes or \"quotes\"."
	result := convertPlainTextToHTML(input)
	
	// Should be equal since we don't escape ' or "
	if result != input {
		t.Errorf("Apostrophes and quotes should not be escaped.\nGot:      %q\nExpected: %q", result, input)
	}
}
