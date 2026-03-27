package docs

import (
	"fmt"
	"strings"
)

// PDF generator — produces a valid PDF document from structured content.
// No external dependencies. Generates PDF 1.4 with embedded Helvetica font.

// pdfDoc builds a PDF document incrementally
type pdfDoc struct {
	objects []string // object contents (1-indexed in PDF)
	offsets []int    // byte offsets of each object
	buf     strings.Builder
}

func newPDFDoc() *pdfDoc {
	return &pdfDoc{}
}

// addObject adds a PDF object and returns its object number (1-based)
func (d *pdfDoc) addObject(content string) int {
	d.objects = append(d.objects, content)
	return len(d.objects)
}

// render produces the final PDF bytes
func (d *pdfDoc) render() []byte {
	d.buf.Reset()

	// Header
	d.buf.WriteString("%PDF-1.4\n")
	// Binary comment to mark as binary
	d.buf.WriteString("%\xe2\xe3\xcf\xd3\n")

	// Write all objects
	d.offsets = make([]int, len(d.objects))
	for i, obj := range d.objects {
		d.offsets[i] = d.buf.Len()
		d.buf.WriteString(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", i+1, obj))
	}

	// Cross-reference table
	xrefOffset := d.buf.Len()
	d.buf.WriteString("xref\n")
	d.buf.WriteString(fmt.Sprintf("0 %d\n", len(d.objects)+1))
	d.buf.WriteString("0000000000 65535 f \n")
	for _, off := range d.offsets {
		d.buf.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
	}

	// Trailer
	d.buf.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\n", len(d.objects)+1))
	d.buf.WriteString(fmt.Sprintf("startxref\n%d\n%%%%EOF\n", xrefOffset))

	return []byte(d.buf.String())
}

// generateWhitepaperPDF creates a PDF from the whitepaper markdown content.
// Parses basic markdown (headings, paragraphs, bold, tables) into PDF text.
func generateWhitepaperPDF(markdown string) []byte {
	doc := newPDFDoc()

	// Parse markdown into lines/blocks
	lines := strings.Split(markdown, "\n")

	type textBlock struct {
		kind string // "title", "h1", "h2", "h3", "para", "table-header", "table-row", "ref", "meta", "space"
		text string
	}

	var blocks []textBlock

	inTable := false
	tableHeaderSeen := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Skip horizontal rules
		if trimmed == "---" {
			inTable = false
			tableHeaderSeen = false
			continue
		}

		// Skip empty lines (produce spacing)
		if trimmed == "" {
			if len(blocks) > 0 && blocks[len(blocks)-1].kind != "space" {
				blocks = append(blocks, textBlock{kind: "space", text: ""})
			}
			inTable = false
			tableHeaderSeen = false
			continue
		}

		// Title (# heading)
		if strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "## ") {
			blocks = append(blocks, textBlock{kind: "title", text: strings.TrimPrefix(trimmed, "# ")})
			continue
		}

		// H2
		if strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "### ") {
			blocks = append(blocks, textBlock{kind: "h2", text: strings.TrimPrefix(trimmed, "## ")})
			continue
		}

		// H3
		if strings.HasPrefix(trimmed, "### ") {
			blocks = append(blocks, textBlock{kind: "h3", text: strings.TrimPrefix(trimmed, "### ")})
			continue
		}

		// Table
		if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") {
			// Skip separator rows (|---|---|)
			inner := strings.Trim(trimmed, "|")
			if strings.Count(inner, "-") > len(inner)/2 {
				continue
			}
			cells := parseTableRow(trimmed)
			if !inTable || !tableHeaderSeen {
				blocks = append(blocks, textBlock{kind: "table-header", text: strings.Join(cells, " | ")})
				inTable = true
				tableHeaderSeen = true
			} else {
				blocks = append(blocks, textBlock{kind: "table-row", text: strings.Join(cells, " | ")})
			}
			continue
		}

		inTable = false
		tableHeaderSeen = false

		// References [N]
		if len(trimmed) > 2 && trimmed[0] == '[' && trimmed[1] >= '0' && trimmed[1] <= '9' {
			blocks = append(blocks, textBlock{kind: "ref", text: trimmed})
			continue
		}

		// Bold-only lines (author, date, etc.)
		if strings.HasPrefix(trimmed, "**") && strings.HasSuffix(trimmed, "**") && strings.Count(trimmed, "**") == 2 {
			inner := trimmed[2 : len(trimmed)-2]
			blocks = append(blocks, textBlock{kind: "meta", text: inner})
			continue
		}

		// Regular paragraph (may span multiple lines)
		para := trimmed
		for i+1 < len(lines) {
			next := strings.TrimSpace(lines[i+1])
			if next == "" || strings.HasPrefix(next, "#") || strings.HasPrefix(next, "|") ||
				strings.HasPrefix(next, "---") || strings.HasPrefix(next, "**") ||
				(len(next) > 2 && next[0] == '[' && next[1] >= '0' && next[1] <= '9') {
				break
			}
			i++
			para += " " + next
		}
		blocks = append(blocks, textBlock{kind: "para", text: cleanMarkdown(para)})
	}

	// Now generate PDF pages from blocks.
	// Page dimensions: A4 (595 x 842 points)
	marginLeft := 60.0
	marginRight := 60.0
	marginTop := 60.0
	marginBottom := 60.0
	usableWidth := 595.0 - marginLeft - marginRight
	pageHeight := 842.0

	// Font sizes
	titleSize := 14.0
	h2Size := 12.0
	h3Size := 11.0
	bodySize := 10.0
	metaSize := 10.0
	refSize := 9.0
	lineSpacing := 1.4

	type pageContent struct {
		stream string
	}

	var pages []pageContent
	var currentStream strings.Builder
	y := pageHeight - marginTop

	newPage := func() {
		if currentStream.Len() > 0 {
			pages = append(pages, pageContent{stream: currentStream.String()})
		}
		currentStream.Reset()
		y = pageHeight - marginTop
		currentStream.WriteString("BT\n")
	}

	endPage := func() {
		currentStream.WriteString("ET\n")
	}

	checkSpace := func(needed float64) {
		if y-needed < marginBottom {
			endPage()
			newPage()
		}
	}

	// Start first page
	currentStream.WriteString("BT\n")

	writeText := func(font string, size float64, text string, indent float64) {
		leading := size * lineSpacing
		maxWidth := usableWidth - indent

		// Word wrap
		wrappedLines := wordWrap(text, size, font, maxWidth)

		for _, wl := range wrappedLines {
			checkSpace(leading)
			escaped := pdfEscape(wl)
			currentStream.WriteString(fmt.Sprintf("/%s %.1f Tf\n", font, size))
			currentStream.WriteString(fmt.Sprintf("1 0 0 1 %.1f %.1f Tm\n", marginLeft+indent, y))
			currentStream.WriteString(fmt.Sprintf("(%s) Tj\n", escaped))
			y -= leading
		}
	}

	for _, block := range blocks {
		switch block.kind {
		case "title":
			checkSpace(titleSize * 3)
			writeText("F1B", titleSize, block.text, 0)
			y -= titleSize * 0.5

		case "meta":
			checkSpace(metaSize * 2)
			writeText("F1", metaSize, block.text, 0)

		case "h2":
			y -= h2Size * 0.5
			checkSpace(h2Size * 3)
			writeText("F1B", h2Size, block.text, 0)
			y -= h2Size * 0.3

		case "h3":
			y -= h3Size * 0.3
			checkSpace(h3Size * 2)
			writeText("F1B", h3Size, block.text, 0)
			y -= h3Size * 0.2

		case "para":
			checkSpace(bodySize * 2)
			writeText("F1", bodySize, block.text, 0)
			y -= bodySize * 0.3

		case "table-header":
			checkSpace(bodySize * 2)
			writeText("F1B", bodySize-1, block.text, 4)

		case "table-row":
			checkSpace(bodySize * 1.5)
			writeText("F1", bodySize-1, block.text, 4)

		case "ref":
			checkSpace(refSize * 1.5)
			writeText("F1", refSize, block.text, 0)

		case "space":
			y -= bodySize * 0.6
		}
	}

	// End last page
	endPage()
	pages = append(pages, pageContent{stream: currentStream.String()})

	// Build PDF objects

	// Object 1: Catalog
	doc.addObject("<< /Type /Catalog /Pages 2 0 R >>")

	// Object 2: Pages (placeholder — updated after page creation)
	pagesObjNum := doc.addObject("") // will be replaced

	// Object 3: Font (Helvetica)
	fontObjNum := doc.addObject("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")

	// Object 4: Font Bold (Helvetica-Bold)
	fontBoldObjNum := doc.addObject("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")

	// Create page objects
	var pageObjNums []int
	for _, pg := range pages {
		// Content stream
		streamContent := pg.stream
		streamObjNum := doc.addObject(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(streamContent), streamContent))

		// Page object
		pageObjNum := doc.addObject(fmt.Sprintf(
			"<< /Type /Page /Parent %d 0 R /MediaBox [0 0 595 842] /Contents %d 0 R /Resources << /Font << /F1 %d 0 R /F1B %d 0 R >> >> >>",
			pagesObjNum, streamObjNum, fontObjNum, fontBoldObjNum,
		))
		pageObjNums = append(pageObjNums, pageObjNum)
	}

	// Update Pages object
	var kids strings.Builder
	for i, pn := range pageObjNums {
		if i > 0 {
			kids.WriteString(" ")
		}
		kids.WriteString(fmt.Sprintf("%d 0 R", pn))
	}
	doc.objects[pagesObjNum-1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), len(pageObjNums))

	return doc.render()
}

// wordWrap splits text into lines that fit within maxWidth at the given font size.
// Uses approximate character widths for Helvetica.
func wordWrap(text string, fontSize float64, font string, maxWidth float64) []string {
	avgCharWidth := fontSize * 0.5 // Helvetica average
	if font == "F1B" {
		avgCharWidth = fontSize * 0.52
	}

	maxChars := int(maxWidth / avgCharWidth)
	if maxChars < 20 {
		maxChars = 20
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	var lines []string
	current := words[0]

	for _, word := range words[1:] {
		if len(current)+1+len(word) > maxChars {
			lines = append(lines, current)
			current = word
		} else {
			current += " " + word
		}
	}
	lines = append(lines, current)

	return lines
}

// pdfEscape escapes special PDF string characters
func pdfEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "(", "\\(")
	s = strings.ReplaceAll(s, ")", "\\)")
	// Replace non-ASCII with approximations
	s = strings.ReplaceAll(s, "\u2014", " -- ")
	s = strings.ReplaceAll(s, "\u2013", " - ")
	s = strings.ReplaceAll(s, "\u2018", "'")
	s = strings.ReplaceAll(s, "\u2019", "'")
	s = strings.ReplaceAll(s, "\u201c", "\"")
	s = strings.ReplaceAll(s, "\u201d", "\"")
	s = strings.ReplaceAll(s, "\u00a3", "GBP ")
	s = strings.ReplaceAll(s, "\u00e9", "e")
	// Remove other non-ASCII
	var clean strings.Builder
	for _, r := range s {
		if r < 128 {
			clean.WriteRune(r)
		}
	}
	return clean.String()
}

// cleanMarkdown strips basic markdown formatting
func cleanMarkdown(s string) string {
	// Remove bold markers
	s = strings.ReplaceAll(s, "**", "")
	// Remove italic markers (single *)
	// Be careful not to strip multiplication
	// Remove links [text](url) -> text
	for {
		start := strings.Index(s, "[")
		if start == -1 {
			break
		}
		mid := strings.Index(s[start:], "](")
		if mid == -1 {
			break
		}
		end := strings.Index(s[start+mid:], ")")
		if end == -1 {
			break
		}
		linkText := s[start+1 : start+mid]
		s = s[:start] + linkText + s[start+mid+end+1:]
	}
	// Remove backticks
	s = strings.ReplaceAll(s, "`", "")
	return s
}

// parseTableRow extracts cell contents from a markdown table row
func parseTableRow(row string) []string {
	row = strings.Trim(row, "|")
	parts := strings.Split(row, "|")
	var cells []string
	for _, p := range parts {
		cell := strings.TrimSpace(p)
		cell = strings.ReplaceAll(cell, "**", "")
		if cell != "" {
			cells = append(cells, cell)
		}
	}
	return cells
}
