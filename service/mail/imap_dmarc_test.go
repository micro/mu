package mail

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	netmail "net/mail"
	"strings"
	"testing"
)

func TestIMAPReportHTMLAndAttachment(t *testing.T) {
	xml := []byte(`<feedback><report_metadata><org_name>Reporter &lt;script&gt;bad&lt;/script&gt;</org_name><report_id>report-1</report_id></report_metadata><policy_published><domain>example.com</domain><p>reject</p></policy_published><record><row><source_ip>192.0.2.1</source_ip><count>12</count><policy_evaluated><disposition>none</disposition></policy_evaluated></row><auth_results><dkim><result>pass</result></dkim><spf><result>fail</result></spf></auth_results></record></feedback>`)
	var gz, zipped bytes.Buffer
	g := gzip.NewWriter(&gz)
	g.Write(xml)
	g.Close()
	z := zip.NewWriter(&zipped)
	f, err := z.Create("report.xml")
	if err != nil {
		t.Fatal(err)
	}
	f.Write(xml)
	z.Close()
	for name, raw := range map[string][]byte{"report.xml": xml, "report.xml.gz": gz.Bytes(), "report.zip": zipped.Bytes()} {
		t.Run(name, func(t *testing.T) {
			m := &Message{ID: "report", FromID: "dmarc@example.com", Body: "[report — not shown]", Attachment: base64.StdEncoding.EncodeToString(raw), AttachmentName: name, AttachmentType: "application/octet-stream"}
			full := imapRender(m)
			parsed, err := netmail.ReadMessage(bytes.NewReader(full))
			if err != nil {
				t.Fatal(err)
			}
			typ, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
			if err != nil || typ != "multipart/mixed" {
				t.Fatalf("MIME type: %s %v", typ, err)
			}
			reader := multipart.NewReader(parsed.Body, params["boundary"])
			first, err := reader.NextRawPart()
			if err != nil {
				t.Fatal(err)
			}
			if first.Header.Get("Content-Type") != "text/html; charset=utf-8" {
				t.Fatal(first.Header)
			}
			body, err := io.ReadAll(first)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"<table", "example.com", "192.0.2.1", "12", "pass", "fail", "&lt;script&gt;"} {
				if !bytes.Contains(body, []byte(want)) {
					t.Errorf("missing %s", want)
				}
			}
			if bytes.Contains(body, []byte("<script>")) || bytes.Contains(body, []byte("not shown")) {
				t.Fatalf("unsafe/placeholder HTML: %s", body)
			}
			if !bytes.Equal(body, imapPart(full, 1)) {
				t.Fatal("BODY[1] differs from MIME part")
			}
			structure := imapBodyStructure(m)
			want := fmt.Sprintf(`("TEXT" "HTML" ("CHARSET" "UTF-8") NIL NIL "8BIT" %d %d)`, len(body), bytes.Count(body, []byte("\r\n")))
			if !strings.Contains(structure, want) {
				t.Fatalf("BODYSTRUCTURE mismatch: %s", structure)
			}
			second, err := reader.NextRawPart()
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := io.ReadAll(second)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(encoded, imapPart(full, 2)) {
				t.Fatal("BODY[2] differs from MIME part")
			}
			decoded, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, bytes.NewReader(encoded)))
			if err != nil || !bytes.Equal(raw, decoded) {
				t.Fatalf("attachment changed: %v", err)
			}
			if !strings.Contains(structure, fmt.Sprintf(`"BASE64" %d`, len(encoded))) {
				t.Fatal("attachment size mismatch")
			}
			if _, err := reader.NextRawPart(); err != io.EOF {
				t.Fatalf("extra part: %v", err)
			}
			if m.Body != "[report — not shown]" {
				t.Fatal("stored message mutated")
			}
			m.Body = "Reporter introduction & context"
			if !strings.Contains(clientBody(m), "Reporter introduction &amp; context") {
				t.Fatal("lost original message text")
			}
		})
	}
}

func TestIMAPReportFallback(t *testing.T) {
	for _, raw := range []string{"ordinary attachment", "<feedback>", "<other/>"} {
		m := &Message{ID: "fallback", Body: "Original\r\nbody", Attachment: base64.StdEncoding.EncodeToString([]byte(raw))}
		if clientBody(m) != m.Body {
			t.Fatal("non-report changed")
		}
		if !strings.Contains(imapBodyStructure(m), `"TEXT" "PLAIN"`) {
			t.Fatal("non-report became HTML")
		}
	}
	m := &Message{Body: "one\r\ntwo\nthree"}
	_, body := imapSplitMessage(imapRender(m))
	if string(body) != "one\r\ntwo\r\nthree\r\n" {
		t.Fatalf("line endings: %q", body)
	}
	if !strings.Contains(imapBodyStructure(m), fmt.Sprintf(`"8BIT" %d 3`, len(body))) {
		t.Fatal("single part size/lines mismatch")
	}
	var b bytes.Buffer
	g := gzip.NewWriter(&b)
	g.Write([]byte(strings.Repeat("x", 5*1024*1024+1)))
	g.Close()
	m.Attachment = base64.StdEncoding.EncodeToString(b.Bytes())
	if clientBody(m) != m.Body {
		t.Fatal("oversized report did not fall back")
	}
}
