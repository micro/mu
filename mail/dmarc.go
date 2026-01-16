package mail

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"mu/app"
)

// extractZipContents extracts all files from a ZIP archive and returns their contents as a string
// Only extracts if sender is a trusted DMARC reporter
func extractZipContents(data []byte, senderEmail string) string {
	// Only auto-extract from trusted DMARC report senders
	trustedSenders := []string{
		"@google.com",
		"@yahoo.com",
		"@outlook.com",
		"@microsoft.com",
		"@amazon.com",
		"@apple.com",
	}

	// Check if sender contains "dmarc" OR is from a trusted domain
	isTrusted := strings.Contains(strings.ToLower(senderEmail), "dmarc")
	if !isTrusted {
		senderLower := strings.ToLower(senderEmail)
		for _, domain := range trustedSenders {
			if strings.HasSuffix(senderLower, domain) {
				isTrusted = true
				break
			}
		}
	}

	if !isTrusted {
		app.Log("mail", "Not extracting ZIP - sender not trusted: %s", senderEmail)
		return "" // Don't auto-extract from unknown senders
	}

	// Size limit: 10MB
	if len(data) > 10*1024*1024 {
		app.Log("mail", "ZIP too large: %d bytes", len(data))
		return ""
	}

	// Log first few bytes for debugging
	if len(data) >= 4 {
		app.Log("mail", "ZIP signature: %02x %02x %02x %02x", data[0], data[1], data[2], data[3])
	}

	// Check if it's actually gzip (DMARC reports are often .xml.gz)
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		app.Log("mail", "Detected gzip format, attempting to decompress")
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			app.Log("mail", "Failed to create gzip reader: %v", err)
			return ""
		}
		defer reader.Close()

		content, err := io.ReadAll(reader)
		if err != nil {
			app.Log("mail", "Failed to read gzip: %v", err)
			return ""
		}

		if isValidUTF8Text(content) {
			app.Log("mail", "Successfully decompressed gzip file (%d bytes)", len(content))
			return string(content)
		}
		app.Log("mail", "Gzip content is not valid text")
		return ""
	}

	reader := bytes.NewReader(data)
	zipReader, err := zip.NewReader(reader, int64(len(data)))
	if err != nil {
		app.Log("mail", "Failed to read ZIP: %v", err)
		return ""
	}

	// Limit number of files
	if len(zipReader.File) > 10 {
		app.Log("mail", "ZIP has too many files: %d", len(zipReader.File))
		return ""
	}

	app.Log("mail", "Extracting ZIP from %s: %d files", senderEmail, len(zipReader.File))

	var result strings.Builder
	filesExtracted := 0
	var singleFileContent string // Store content if it's a single file

	for i, file := range zipReader.File {
		// Limit individual file size: 5MB
		if file.UncompressedSize64 > 5*1024*1024 {
			app.Log("mail", "Skipping large file: %s (%d bytes)", file.Name, file.UncompressedSize64)
			continue
		}

		rc, err := file.Open()
		if err != nil {
			if i > 0 {
				result.WriteString("\n\n" + strings.Repeat("=", 80) + "\n\n")
			}
			result.WriteString(fmt.Sprintf("File: %s (%d bytes)\n", file.Name, file.UncompressedSize64))
			result.WriteString(strings.Repeat("-", 80) + "\n\n")
			result.WriteString(fmt.Sprintf("Error opening file: %v\n", err))
			app.Log("mail", "Failed to open file %s: %v", file.Name, err)
			continue
		}

		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			if i > 0 {
				result.WriteString("\n\n" + strings.Repeat("=", 80) + "\n\n")
			}
			result.WriteString(fmt.Sprintf("File: %s (%d bytes)\n", file.Name, file.UncompressedSize64))
			result.WriteString(strings.Repeat("-", 80) + "\n\n")
			result.WriteString(fmt.Sprintf("Error reading file: %v\n", err))
			app.Log("mail", "Failed to read file %s: %v", file.Name, err)
			continue
		}

		// Only display text content (XML, TXT, etc) - never execute or render HTML
		if isValidUTF8Text(content) {
			filesExtracted++

			// If single file, store raw content without headers
			if len(zipReader.File) == 1 {
				singleFileContent = string(content)
				app.Log("mail", "Extracted single text file: %s (%d bytes)", file.Name, len(content))
			} else {
				// Multiple files - add headers
				if i > 0 {
					result.WriteString("\n\n" + strings.Repeat("=", 80) + "\n\n")
				}
				result.WriteString(fmt.Sprintf("File: %s (%d bytes)\n", file.Name, file.UncompressedSize64))
				result.WriteString(strings.Repeat("-", 80) + "\n\n")
				result.WriteString(string(content))
				app.Log("mail", "Extracted text file: %s (%d bytes)", file.Name, len(content))
			}
		} else {
			if i > 0 {
				result.WriteString("\n\n" + strings.Repeat("=", 80) + "\n\n")
			}
			result.WriteString(fmt.Sprintf("File: %s (%d bytes)\n", file.Name, file.UncompressedSize64))
			result.WriteString(strings.Repeat("-", 80) + "\n\n")
			result.WriteString(fmt.Sprintf("[Binary file, %d bytes - not displayed]\n", len(content)))
			app.Log("mail", "Skipped binary file: %s", file.Name)
		}
	}

	if filesExtracted == 0 {
		app.Log("mail", "No text files extracted from ZIP")
		return ""
	}

	app.Log("mail", "Successfully extracted %d files from ZIP", filesExtracted)

	// For single file ZIPs (like DMARC reports), return raw content
	if len(zipReader.File) == 1 && singleFileContent != "" {
		return singleFileContent
	}

	if result.Len() == 0 {
		return ""
	}

	return result.String()
}

// DMARC XML structures
type DMARCReport struct {
	XMLName         xml.Name        `xml:"feedback"`
	ReportMetadata  ReportMetadata  `xml:"report_metadata"`
	PolicyPublished PolicyPublished `xml:"policy_published"`
	Records         []Record        `xml:"record"`
}

type ReportMetadata struct {
	OrgName   string    `xml:"org_name"`
	Email     string    `xml:"email"`
	ReportID  string    `xml:"report_id"`
	DateRange DateRange `xml:"date_range"`
}

type DateRange struct {
	Begin int64 `xml:"begin"`
	End   int64 `xml:"end"`
}

type PolicyPublished struct {
	Domain string `xml:"domain"`
	ADKIM  string `xml:"adkim"`
	ASPF   string `xml:"aspf"`
	P      string `xml:"p"`
	SP     string `xml:"sp"`
	Pct    int    `xml:"pct"`
}

type Record struct {
	Row         Row         `xml:"row"`
	Identifiers Identifiers `xml:"identifiers"`
	AuthResults AuthResults `xml:"auth_results"`
}

type Row struct {
	SourceIP        string          `xml:"source_ip"`
	Count           int             `xml:"count"`
	PolicyEvaluated PolicyEvaluated `xml:"policy_evaluated"`
}

type PolicyEvaluated struct {
	Disposition string `xml:"disposition"`
	DKIM        string `xml:"dkim"`
	SPF         string `xml:"spf"`
}

type Identifiers struct {
	HeaderFrom string `xml:"header_from"`
}

type AuthResults struct {
	DKIM []DKIMResult `xml:"dkim"`
	SPF  []SPFResult  `xml:"spf"`
}

type DKIMResult struct {
	Domain   string `xml:"domain"`
	Result   string `xml:"result"`
	Selector string `xml:"selector"`
}

type SPFResult struct {
	Domain string `xml:"domain"`
	Result string `xml:"result"`
}

// renderDMARCReport parses DMARC XML and renders it as HTML tables
func renderDMARCReport(xmlData string) string {
	app.Log("mail", "renderDMARCReport called with %d bytes, first 200 chars: %s", len(xmlData), xmlData[:min(200, len(xmlData))])

	var report DMARCReport
	if err := xml.Unmarshal([]byte(xmlData), &report); err != nil {
		// Not a DMARC report or invalid XML - return empty to fall back to raw display
		app.Log("mail", "Failed to parse as DMARC report: %v", err)
		return ""
	}

	app.Log("mail", "Successfully parsed DMARC report from %s", report.ReportMetadata.OrgName)

	var html strings.Builder

	// Report metadata
	html.WriteString(`<div style="margin-bottom: 20px;">`)
	html.WriteString(fmt.Sprintf(`<h4 style="margin: 0 0 10px 0;">DMARC Report from %s</h4>`, report.ReportMetadata.OrgName))
	html.WriteString(`<table style="border-collapse: collapse; width: 100%; font-size: 13px;">`)
	html.WriteString(fmt.Sprintf(`<tr><td style="padding: 4px 8px; background: #f5f5f5;"><strong>Report ID:</strong></td><td style="padding: 4px 8px;">%s</td></tr>`, report.ReportMetadata.ReportID))
	html.WriteString(fmt.Sprintf(`<tr><td style="padding: 4px 8px; background: #f5f5f5;"><strong>Domain:</strong></td><td style="padding: 4px 8px;">%s</td></tr>`, report.PolicyPublished.Domain))
	html.WriteString(fmt.Sprintf(`<tr><td style="padding: 4px 8px; background: #f5f5f5;"><strong>Policy:</strong></td><td style="padding: 4px 8px;">%s</td></tr>`, report.PolicyPublished.P))
	html.WriteString(`</table></div>`)

	// Records table
	if len(report.Records) > 0 {
		html.WriteString(`<h4 style="margin: 0 0 10px 0;">Email Results</h4>`)
		html.WriteString(`<table style="border-collapse: collapse; width: 100%; font-size: 12px; border: 1px solid #ddd;">`)
		html.WriteString(`<thead><tr style="background: #f5f5f5;">`)
		html.WriteString(`<th style="padding: 8px; text-align: left; border: 1px solid #ddd;">Source IP</th>`)
		html.WriteString(`<th style="padding: 8px; text-align: left; border: 1px solid #ddd;">Count</th>`)
		html.WriteString(`<th style="padding: 8px; text-align: left; border: 1px solid #ddd;">DKIM</th>`)
		html.WriteString(`<th style="padding: 8px; text-align: left; border: 1px solid #ddd;">SPF</th>`)
		html.WriteString(`<th style="padding: 8px; text-align: left; border: 1px solid #ddd;">Disposition</th>`)
		html.WriteString(`</tr></thead><tbody>`)

		for _, record := range report.Records {
			dkimResult := "none"
			if len(record.AuthResults.DKIM) > 0 {
				dkimResult = record.AuthResults.DKIM[0].Result
			}
			spfResult := "none"
			if len(record.AuthResults.SPF) > 0 {
				spfResult = record.AuthResults.SPF[0].Result
			}

			// Color code results
			dkimColor := "#d4edda" // green
			if dkimResult != "pass" {
				dkimColor = "#f8d7da" // red
			}
			spfColor := "#d4edda"
			if spfResult != "pass" {
				spfColor = "#f8d7da"
			}

			html.WriteString(`<tr>`)
			html.WriteString(fmt.Sprintf(`<td style="padding: 8px; border: 1px solid #ddd;">%s</td>`, record.Row.SourceIP))
			html.WriteString(fmt.Sprintf(`<td style="padding: 8px; border: 1px solid #ddd;">%d</td>`, record.Row.Count))
			html.WriteString(fmt.Sprintf(`<td style="padding: 8px; border: 1px solid #ddd; background: %s;">%s</td>`, dkimColor, dkimResult))
			html.WriteString(fmt.Sprintf(`<td style="padding: 8px; border: 1px solid #ddd; background: %s;">%s</td>`, spfColor, spfResult))
			html.WriteString(fmt.Sprintf(`<td style="padding: 8px; border: 1px solid #ddd;">%s</td>`, record.Row.PolicyEvaluated.Disposition))
			html.WriteString(`</tr>`)
		}

		html.WriteString(`</tbody></table>`)
	}

	result := html.String()
	app.Log("mail", "renderDMARCReport returning %d bytes of HTML", len(result))
	return result
}
