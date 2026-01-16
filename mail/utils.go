package mail

import (
	"fmt"
	"html"
	"io"
	"mime/quotedprintable"
	"strings"

	"mu/app"
)

// looksLikeBase64 checks if a string appears to be base64 encoded
func looksLikeBase64(s string) bool {
	s = strings.TrimSpace(s)

	// Must be reasonable length (not empty, not too short)
	if len(s) < 20 {
		return false
	}

	// Base64 strings should be mostly base64 characters (a-zA-Z0-9+/=)
	validChars := 0
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '+' || c == '/' || c == '=' || c == '\n' || c == '\r' {
			validChars++
		}
	}

	// If more than 90% of characters are valid base64 chars, likely base64
	return float64(validChars)/float64(len(s)) > 0.9
}

// isValidUTF8Text checks if decoded bytes are valid UTF-8 text
func isValidUTF8Text(data []byte) bool {
	// Check if it's valid UTF-8
	if !strings.HasPrefix(string(data), "\xff\xfe") && !strings.HasPrefix(string(data), "\xfe\xff") {
		text := string(data)
		// Should contain mostly printable characters
		printable := 0
		for _, r := range text {
			if r >= 32 || r == '\t' || r == '\n' || r == '\r' {
				printable++
			}
		}
		// If more than 90% printable, consider it valid text
		if len(text) > 0 && float64(printable)/float64(len(text)) > 0.9 {
			return true
		}
	}
	return false
}

// looksLikeMarkdown checks if text contains markdown formatting
func looksLikeMarkdown(text string) bool {
	// Check for definitive markdown patterns (require full syntax)
	definitivePatterns := []string{
		"**",  // bold (needs two asterisks)
		"__",  // bold (needs two underscores)
		"```", // code block
		"- ",  // unordered list
		"* ",  // unordered list (at start)
	}

	for _, pattern := range definitivePatterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}

	// Check for markdown links [text](url) - need both parts
	if strings.Contains(text, "[") && strings.Contains(text, "](") {
		return true
	}

	// Check for headers (# at start of line)
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "# ") {
			return true
		}
	}

	return false
}

// stripHTMLTags removes HTML tags from a string, leaving only text content
// This is used for email previews to prevent HTML from breaking the layout
func stripHTMLTags(s string) string {
	// First, convert block-level HTML elements to newlines to preserve structure
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")
	s = strings.ReplaceAll(s, "</p>", "\n")
	s = strings.ReplaceAll(s, "</div>", "\n")
	s = strings.ReplaceAll(s, "</blockquote>", "\n")
	s = strings.ReplaceAll(s, "</li>", "\n")
	s = strings.ReplaceAll(s, "</tr>", "\n")
	s = strings.ReplaceAll(s, "</h1>", "\n")
	s = strings.ReplaceAll(s, "</h2>", "\n")
	s = strings.ReplaceAll(s, "</h3>", "\n")

	// Simple tag stripper - removes anything between < and >
	var result strings.Builder
	inTag := false

	for _, char := range s {
		if char == '<' {
			inTag = true
			continue
		}
		if char == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(char)
		}
	}

	// Decode common HTML entities
	text := result.String()
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")

	// Trim leading whitespace from each line to remove HTML indentation
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimLeft(line, " \t")
	}
	text = strings.Join(lines, "\n")

	return text
}

// looksLikeQuotedPrintable detects if content appears to be quoted-printable encoded
func looksLikeQuotedPrintable(text string) bool {
	// Count lines ending with = (soft line breaks)
	lines := strings.Split(text, "\n")
	softBreaks := 0

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.HasSuffix(line, "=") {
			softBreaks++
		}
	}

	// If more than 5 lines end with =, it's likely quoted-printable
	return softBreaks > 5
}

// looksLikeHTML detects if content is HTML (from external emails)
func looksLikeHTML(text string) bool {
	text = strings.TrimSpace(text)

	// Check for common HTML indicators
	htmlIndicators := []string{
		"<!DOCTYPE html",
		"<!doctype html",
		"<html",
		"<HTML",
	}

	for _, indicator := range htmlIndicators {
		if strings.HasPrefix(strings.ToLower(text), strings.ToLower(indicator)) {
			return true
		}
	}

	// Check if it starts with common HTML tags
	if strings.HasPrefix(text, "<") {
		// Look for typical HTML structure tags
		htmlTags := []string{"<html", "<head", "<body", "<div", "<p", "<table", "<span"}
		textLower := strings.ToLower(text)
		for _, tag := range htmlTags {
			if strings.HasPrefix(textLower, tag) {
				return true
			}
		}
	}

	return false
}

// linkifyURLs converts URLs in text to clickable links and preserves line breaks
func linkifyURLs(text string) string {
	result := ""
	lastIndex := 0

	for i := 0; i < len(text); i++ {
		// Check for http:// or https://
		if strings.HasPrefix(text[i:], "http://") || strings.HasPrefix(text[i:], "https://") || strings.HasPrefix(text[i:], "www.") {
			// Add text before the URL
			result += html.EscapeString(text[lastIndex:i])

			// Find the end of the URL
			end := i
			for end < len(text) && !isURLTerminator(text[end]) {
				end++
			}

			url := text[i:end]
			// Add http:// prefix for www. URLs
			href := url
			if strings.HasPrefix(url, "www.") {
				href = "http://" + url
			}

			// Create clickable link
			result += fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer" style="color: #0066cc; text-decoration: underline;">%s</a>`, href, html.EscapeString(url))

			lastIndex = end
			i = end - 1 // -1 because loop will increment
		}
	}

	// Add remaining text
	result += html.EscapeString(text[lastIndex:])

	// Convert newlines to <br> tags for proper display
	result = strings.ReplaceAll(result, "\r\n", "<br>")
	result = strings.ReplaceAll(result, "\n", "<br>")

	return result
}

// isURLTerminator checks if a character ends a URL
func isURLTerminator(c byte) bool {
	return c == ' ' || c == '\n' || c == '\r' || c == '\t' || c == '<' || c == '>' ||
		c == '"' || c == '\'' || c == ')' || c == ']' || c == '}' || c == ',' || c == ';'
}

// renderEmailBody processes email body - renders markdown if detected, otherwise linkifies URLs
func renderEmailBody(body string, isAttachment bool) string {
	if isAttachment {
		return body
	}

	// Check if body contains PGP signed message
	if strings.Contains(body, "-----BEGIN PGP SIGNATURE-----") {
		// For signed messages, extract the cleartext before the signature
		sigIdx := strings.Index(body, "-----BEGIN PGP SIGNATURE-----")
		if sigIdx > 0 {
			// The actual message is before the signature
			cleartext := strings.TrimSpace(body[:sigIdx])
			app.Log("mail", "Extracted cleartext from PGP signed message")
			body = cleartext
		}
	}

	// Check if body contains PGP encrypted message
	if strings.Contains(body, "-----BEGIN PGP MESSAGE-----") {
		decrypted, err := decryptPGPMessage(body)
		if err != nil {
			app.Log("mail", "PGP decryption failed: %v", err)
			// Return original body with error notice
			return fmt.Sprintf(`<div style="background: #fff3cd; padding: 10px; border-radius: 5px; margin-bottom: 10px; border-left: 4px solid #ffc107;">
				<strong>🔒 PGP Encrypted Message</strong><br>
				Decryption failed: %s
			</div>
			<pre style="background: #f5f5f5; padding: 10px; border-radius: 5px; overflow-x: auto; font-family: monospace; font-size: 12px;">%s</pre>`,
				html.EscapeString(err.Error()),
				html.EscapeString(body))
		}
		// Successfully decrypted - process the decrypted content
		body = decrypted
		app.Log("mail", "Successfully decrypted PGP message")
	}

	// Check if body is HTML (from external emails)
	if looksLikeHTML(body) {
		// Extract body content and clean up email-specific HTML
		return extractHTMLBody(body)
	}

	// Check if body looks like markdown
	if looksLikeMarkdown(body) {
		// Render markdown to HTML
		rendered := app.RenderString(body)

		// Clean up excessive whitespace while preserving HTML structure
		for strings.Contains(rendered, "\n\n\n") {
			rendered = strings.ReplaceAll(rendered, "\n\n\n", "\n\n")
		}

		// Remove newlines between tags
		rendered = strings.ReplaceAll(rendered, ">\n<", "><")
		rendered = strings.ReplaceAll(rendered, ">\n\n<", "><")

		return rendered
	}

	// Otherwise just linkify URLs
	return linkifyURLs(body)
}

// extractHTMLBody extracts and cleans content from HTML email
func extractHTMLBody(htmlContent string) string {
	// Detect and decode quoted-printable encoding
	// Signs: contains =3D (encoded =), =\n (soft line breaks), or has many = signs at line ends
	isQuotedPrintable := strings.Contains(htmlContent, "=3D") ||
		strings.Contains(htmlContent, "=\n") ||
		strings.Contains(htmlContent, "=\r\n") ||
		looksLikeQuotedPrintable(htmlContent)

	if isQuotedPrintable {
		reader := quotedprintable.NewReader(strings.NewReader(htmlContent))
		if decoded, err := io.ReadAll(reader); err == nil {
			htmlContent = string(decoded)
		}
	}

	// Remove Outlook/MSO conditional comments (they break rendering)
	for strings.Contains(htmlContent, "<!--[if") {
		start := strings.Index(htmlContent, "<!--[if")
		if start == -1 {
			break
		}
		end := strings.Index(htmlContent[start:], "<![endif]-->")
		if end == -1 {
			break
		}
		htmlContent = htmlContent[:start] + htmlContent[start+end+12:]
	}

	// Return the HTML as-is - let the email's own styling work
	return strings.TrimSpace(htmlContent)
}

// convertPlainTextToHTML converts plain text to HTML for email
// Only escapes < > & characters, preserves apostrophes and quotes for natural text
func convertPlainTextToHTML(text string) string {
	// Use html.EscapeString for proper escaping, then selectively unescape quotes and apostrophes
	// This is more maintainable than manual escaping
	escaped := html.EscapeString(text)

	// Unescape apostrophes and double quotes - they're safe in HTML content
	escaped = strings.ReplaceAll(escaped, "&#39;", "'")
	escaped = strings.ReplaceAll(escaped, "&#34;", "\"")

	// Convert newlines to <br> tags
	escaped = strings.ReplaceAll(escaped, "\n", "<br>")

	return escaped
}
