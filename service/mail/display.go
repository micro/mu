package mail

// What a stored message looks like on a page.
//
// # Why this is a function
//
// It was not one, and that is the whole reason /inbox never rendered a DMARC
// table. The same hundred and forty lines of decoding lived twice in mail.go —
// once for the message being viewed and once inside the loop that renders the
// thread — and the first copy was dead: it decoded a body into a variable that
// nothing read afterwards. Of the two copies only one ever ran, and neither
// could be called from outside the handler it sat in.
//
// So the inbox was not a second renderer written wrong. There was no renderer
// to call. Outside this package the only thing available is the text, and the
// only safe thing to do with text is escape it — which turns a DMARC report's
// table into the word "table" and some angle brackets. No fix to the inbox
// reaches this code while this code is inline in an HTTP handler.
//
// Anything needing the same answer calls this. Two pages that disagree is now
// a bug with one cause instead of a diff between two copies.
//
// # What it handles
//
// Mail does not arrive as prose. It arrives as MIME with headers inside the
// body, quoted-printable, base64, gzip, a ZIP with XML in it, and HTML that has
// to be extracted rather than escaped. DMARC reports are most of those at once,
// which is why they are the ones somebody notices first.

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"strings"
)

// Rendered is the message body as HTML, ready to put on a page.
//
// Safe to write unescaped: what is not already HTML goes through
// renderEmailBody, and what is has been through extractHTMLBody, which is
// where the cleaning lives.
func Rendered(msg *Message) string {
	body := msg.Body
	isAttachment := false

	// First, check if body contains raw MIME content with headers
	if strings.Contains(body, "Content-Type:") || strings.Contains(body, "content-type:") {
		if extracted := extractMIMEBody(body); extracted != body {
			body = extracted
		}
	}

	// Check for gzip or ZIP file
	trimmedBody := strings.TrimSpace(body)

	// Check for mixed content: base64 gzip data followed by MIME multipart markers
	// This happens with some DMARC reports from Microsoft
	if strings.Contains(trimmedBody, "[multipart/") || strings.Contains(trimmedBody, "\n--") {
		var gzipPart string
		if idx := strings.Index(trimmedBody, "\n\n[multipart/"); idx > 0 {
			gzipPart = strings.TrimSpace(trimmedBody[:idx])
		} else if idx := strings.Index(trimmedBody, "\n[multipart/"); idx > 0 {
			gzipPart = strings.TrimSpace(trimmedBody[:idx])
		} else if idx := strings.Index(trimmedBody, "[multipart/"); idx > 0 {
			gzipPart = strings.TrimSpace(trimmedBody[:idx])
		} else if idx := strings.Index(trimmedBody, "\n--"); idx > 0 {
			gzipPart = strings.TrimSpace(trimmedBody[:idx])
		}

		if gzipPart != "" && looksLikeBase64(gzipPart) {
			if decoded, err := base64.StdEncoding.DecodeString(gzipPart); err == nil {
				if len(decoded) >= 2 && decoded[0] == 0x1f && decoded[1] == 0x8b {
					if reader, err := gzip.NewReader(bytes.NewReader(decoded)); err == nil {
						if content, err := io.ReadAll(reader); err == nil {
							reader.Close()
							if isValidUTF8Text(content) {
								if dmarcHTML := renderDMARCReport(string(content)); dmarcHTML != "" {
									body = dmarcHTML
									isAttachment = true
								} else {
									body = fmt.Sprintf(`<pre class="code-block-sm">%s</pre>`, html.EscapeString(string(content)))
								}
								goto threadSkipBodyProcessing
							}
						}
					}
				}
			}
		}
	}

	if len(trimmedBody) >= 2 && trimmedBody[0] == 0x1f && trimmedBody[1] == 0x8b {
		// Gzip compressed - decompress and display
		if reader, err := gzip.NewReader(strings.NewReader(trimmedBody)); err == nil {
			if content, err := io.ReadAll(reader); err == nil {
				reader.Close()
				if isValidUTF8Text(content) {
					body = fmt.Sprintf(`<pre class="code-block-sm">%s</pre>`, html.EscapeString(string(content)))
				}
			}
		}
	} else if len(trimmedBody) >= 2 && trimmedBody[0] == 'P' && trimmedBody[1] == 'K' {
		// ZIP file - try to extract
		if extracted := extractZipContents([]byte(trimmedBody), msg.FromID); extracted != "" {
			// Try to render as DMARC report
			if dmarcHTML := renderDMARCReport(extracted); dmarcHTML != "" {
				body = dmarcHTML
				isAttachment = true // Skip linkifyURLs for pre-rendered HTML
			} else {
				body = fmt.Sprintf(`<pre class="code-block-sm">%s</pre>`, html.EscapeString(extracted))
			}
		} else {
			// Extraction failed - show download link
			isAttachment = true
			attachName := "attachment.zip"
			if strings.Contains(strings.ToLower(msg.FromID), "dmarc") {
				attachName = "dmarc-report.zip"
			}
			body = fmt.Sprintf(`📎 <a href="/mail?action=download_attachment&msg_id=%s" download="%s">%s</a>`, msg.ID, attachName, attachName)
		}
	} else if looksLikeBase64(body) {
		if decoded, err := base64.StdEncoding.DecodeString(trimmedBody); err == nil {
			// Check if decoded data is gzip
			if len(decoded) >= 2 && decoded[0] == 0x1f && decoded[1] == 0x8b {
				if reader, err := gzip.NewReader(bytes.NewReader(decoded)); err == nil {
					if content, err := io.ReadAll(reader); err == nil {
						reader.Close()
						if isValidUTF8Text(content) {
							body = fmt.Sprintf(`<pre class="code-block-sm">%s</pre>`, html.EscapeString(string(content)))
						}
					}
				}
			} else if len(decoded) >= 2 && decoded[0] == 'P' && decoded[1] == 'K' {
				// ZIP file - try to extract
				if extracted := extractZipContents(decoded, msg.FromID); extracted != "" {
					// Try to render as DMARC report
					if dmarcHTML := renderDMARCReport(extracted); dmarcHTML != "" {
						body = dmarcHTML
						isAttachment = true // Skip linkifyURLs for pre-rendered HTML
					} else {
						body = fmt.Sprintf(`<pre class="code-block-sm">%s</pre>`, html.EscapeString(extracted))
					}
				} else {
					// Extraction failed - show download link
					isAttachment = true
					attachName := "attachment.zip"
					if strings.Contains(strings.ToLower(msg.FromID), "dmarc") {
						attachName = "dmarc-report.zip"
					}
					body = fmt.Sprintf(`📎 <a href="/mail?action=download_attachment&msg_id=%s" download="%s">%s</a>`, msg.ID, attachName, attachName)
				}
			} else if isValidUTF8Text(decoded) {
				body = string(decoded)
			}
		}
	}

threadSkipBodyProcessing:
	// An attachment stored beside the body wins here too.
	//
	// This loop is a second body renderer, and it only ever knew the old
	// scheme: everything above sniffs msg.Body for base64, gzip and zip
	// magic bytes, because that is where attachments used to live. When
	// they moved out of the body, the single-message view was taught to
	// read them and this was not — so a DMARC report shown inside a
	// thread, which is how a conversation of one is shown, ignored the
	// bytes entirely and printed the line describing them.
	//
	// It is why the report rendered nothing and said nothing: the note
	// about bytes that were never kept lives in the other block, so this
	// path could not even report its own failure.
	if rendered, _ := renderStoredAttachment(msg); rendered != "" {
		body, isAttachment = rendered, true
	} else if describedNothing(msg) {
		body += "\n\n(The attachment itself was not kept — this message " +
			"arrived before they were stored. A new report supersedes it.)"
	}

	// Process email body - renders markdown if detected, otherwise linkifies URLs
	body = renderEmailBody(body, isAttachment)
	return body
}
