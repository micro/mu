package mail

import (
	"strings"
	"testing"
)

func TestEmailHTMLRenderingFlow(t *testing.T) {
	// Test the complete flow of email HTML rendering

	t.Run("Plain text with apostrophes and newlines", func(t *testing.T) {
		// Simulate user input from compose form
		userInput := "Hello!\nIt's a great day.\nDon't you think?"

		// Convert to HTML (what happens in the handler)
		bodyHTML := convertPlainTextToHTML(userInput)

		// Verify apostrophes are preserved (not escaped)
		if strings.Contains(bodyHTML, "&#39;") || strings.Contains(bodyHTML, "&apos;") {
			t.Errorf("Apostrophes should not be HTML-escaped, got: %s", bodyHTML)
		}

		// Verify newlines are converted to <br>
		expectedBRCount := strings.Count(userInput, "\n")
		actualBRCount := strings.Count(bodyHTML, "<br>")
		if expectedBRCount != actualBRCount {
			t.Errorf("Expected %d <br> tags, got %d. Result: %s", expectedBRCount, actualBRCount, bodyHTML)
		}

		// Verify the actual HTML content
		expectedHTML := "Hello!<br>It's a great day.<br>Don't you think?"
		if bodyHTML != expectedHTML {
			t.Errorf("Expected: %q\nGot: %q", expectedHTML, bodyHTML)
		}
	})

	t.Run("Text with HTML special characters", func(t *testing.T) {
		userInput := "5 < 10 and 10 > 5\n& this is important!"

		bodyHTML := convertPlainTextToHTML(userInput)

		// Verify < > & are escaped
		if !strings.Contains(bodyHTML, "&lt;") {
			t.Error("< should be escaped to &lt;")
		}
		if !strings.Contains(bodyHTML, "&gt;") {
			t.Error("> should be escaped to &gt;")
		}
		if !strings.Contains(bodyHTML, "&amp;") {
			t.Error("& should be escaped to &amp;")
		}

		expectedHTML := "5 &lt; 10 and 10 &gt; 5<br>&amp; this is important!"
		if bodyHTML != expectedHTML {
			t.Errorf("Expected: %q\nGot: %q", expectedHTML, bodyHTML)
		}
	})

	t.Run("Text with quotes", func(t *testing.T) {
		userInput := `He said "hello" to me`

		bodyHTML := convertPlainTextToHTML(userInput)

		// Verify quotes are NOT escaped (they're safe in HTML content)
		if strings.Contains(bodyHTML, "&quot;") || strings.Contains(bodyHTML, "&#34;") {
			t.Errorf("Quotes should not be HTML-escaped in content, got: %s", bodyHTML)
		}

		// Should be identical since no special chars need escaping
		if bodyHTML != userInput {
			t.Errorf("Expected: %q\nGot: %q", userInput, bodyHTML)
		}
	})

	t.Run("Empty input", func(t *testing.T) {
		userInput := ""
		bodyHTML := convertPlainTextToHTML(userInput)
		if bodyHTML != "" {
			t.Errorf("Empty input should produce empty output, got: %q", bodyHTML)
		}
	})

	t.Run("Real world example", func(t *testing.T) {
		userInput := "Hi John,\n\nIt's great to hear from you!\n\nI wanted to let you know that 2 < 3 & 3 > 2.\n\nBest regards,\nJane"

		bodyHTML := convertPlainTextToHTML(userInput)

		// Verify structure
		lines := strings.Split(userInput, "\n")
		brCount := strings.Count(bodyHTML, "<br>")
		if brCount != len(lines)-1 {
			t.Errorf("Expected %d <br> tags, got %d", len(lines)-1, brCount)
		}

		// Verify apostrophes preserved
		if !strings.Contains(bodyHTML, "It's") {
			t.Error("Apostrophe in It's should be preserved")
		}

		// Verify < > & escaped
		if !strings.Contains(bodyHTML, "&lt;") || !strings.Contains(bodyHTML, "&gt;") || !strings.Contains(bodyHTML, "&amp;") {
			t.Error("Special HTML characters should be escaped")
		}

		expectedHTML := "Hi John,<br><br>It's great to hear from you!<br><br>I wanted to let you know that 2 &lt; 3 &amp; 3 &gt; 2.<br><br>Best regards,<br>Jane"
		if bodyHTML != expectedHTML {
			t.Errorf("Expected:\n%q\n\nGot:\n%q", expectedHTML, bodyHTML)
		}
	})
}

func TestStripHTMLTags(t *testing.T) {
	// Test that stripHTMLTags properly removes HTML for previews

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple HTML with br tags",
			input:    "Hello!<br>It's working.",
			expected: "Hello!\nIt's working.",
		},
		{
			name:     "HTML with escaped entities",
			input:    "5 &lt; 10 and 10 &gt; 5",
			expected: "5 < 10 and 10 > 5",
		},
		{
			name:     "Complex HTML with multiple tags",
			input:    "<div>Hello</div><br><p>World</p>",
			expected: "Hello\n\nWorld\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripHTMLTags(tt.input)
			if result != tt.expected {
				t.Errorf("Expected: %q\nGot: %q", tt.expected, result)
			}
		})
	}
}
