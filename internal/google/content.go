package google

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"golang.org/x/net/html"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const GmailScope = "https://www.googleapis.com/auth/gmail.readonly"
const DriveScope = "https://www.googleapis.com/auth/drive.readonly"
const contentLimit = 1 << 20

// readAPI keeps credentials within this package and bounds provider responses.
// The endpoint is constructed here, never supplied by a caller or a file URL.
func readAPI(ctx context.Context, owner, scope, endpoint string) ([]byte, error) {
	if owner == "" || !HasScope(owner, scope) {
		return nil, fmt.Errorf("connect %s in Account before using it", Label(scope))
	}
	token, err := accessToken(owner, scope)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Google could not be reached")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("Google access expired; reconnect in Account")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google returned HTTP %d; check your connection and enabled APIs", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, contentLimit+1))
	if err != nil {
		return nil, err
	}
	if len(b) > contentLimit {
		return nil, fmt.Errorf("Google response exceeds the 1 MiB reading limit; open it in Google instead")
	}
	return b, nil
}
func readJSON(ctx context.Context, owner, scope, endpoint string, out any) error {
	b, err := readAPI(ctx, owner, scope, endpoint)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}
func boundedLimit(n int) int {
	if n <= 0 {
		return 10
	}
	if n > 20 {
		return 20
	}
	return n
}
func validID(id string) bool {
	if id == "" || len(id) > 256 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

type GmailMessage struct {
	ID       string `json:"id"`
	ThreadID string `json:"thread_id"`
	Subject  string `json:"subject"`
	From     string `json:"from"`
	To       string `json:"to"`
	Date     string `json:"date"`
	Snippet  string `json:"snippet"`
	Text     string `json:"text,omitempty"`
	URL      string `json:"url"`
}
type GmailPage struct {
	Messages []GmailMessage `json:"messages"`
	NextPage string         `json:"next_page,omitempty"`
}
type gmailPart struct {
	MimeType string `json:"mimeType"`
	Filename string `json:"filename"`
	Headers  []struct {
		Name  string
		Value string
	} `json:"headers"`
	Body struct {
		Data string `json:"data"`
	} `json:"body"`
	Parts []gmailPart `json:"parts"`
}

func plainParts(p gmailPart) string {
	if p.Filename != "" {
		return ""
	}
	if p.MimeType == "text/plain" {
		b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(p.Body.Data, "="))
		if err == nil {
			return string(b)
		}
	}
	var parts []string
	for _, part := range p.Parts {
		if s := plainParts(part); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n")
}
func htmlParts(p gmailPart) string {
	if p.Filename != "" {
		return ""
	}
	if p.MimeType == "text/html" {
		b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(p.Body.Data, "="))
		if err != nil {
			return ""
		}
		doc, err := html.Parse(strings.NewReader(string(b)))
		if err != nil {
			return ""
		}
		var out strings.Builder
		var visit func(*html.Node)
		visit = func(n *html.Node) {
			if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "noscript") {
				return
			}
			if n.Type == html.TextNode {
				out.WriteString(n.Data)
				out.WriteByte(' ')
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				visit(c)
			}
		}
		visit(doc)
		return strings.Join(strings.Fields(out.String()), " ")
	}
	var parts []string
	for _, part := range p.Parts {
		if text := htmlParts(part); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func gmailMessage(ctx context.Context, owner, id, format string) (GmailMessage, error) {
	var result GmailMessage
	if !validID(id) {
		return result, fmt.Errorf("invalid Gmail message id")
	}
	var raw struct {
		ID       string    `json:"id"`
		ThreadID string    `json:"threadId"`
		Snippet  string    `json:"snippet"`
		Payload  gmailPart `json:"payload"`
	}
	q := url.Values{"format": {format}}
	if format == "metadata" {
		q["metadataHeaders"] = []string{"Subject", "From", "To", "Date"}
	}
	err := readJSON(ctx, owner, GmailScope, "https://gmail.googleapis.com/gmail/v1/users/me/messages/"+id+"?"+q.Encode(), &raw)
	if err != nil {
		return result, err
	}
	result = GmailMessage{ID: raw.ID, ThreadID: raw.ThreadID, Snippet: raw.Snippet, URL: "https://mail.google.com/mail/u/0/#all/" + url.PathEscape(raw.ThreadID)}
	for _, h := range raw.Payload.Headers {
		switch strings.ToLower(h.Name) {
		case "subject":
			result.Subject = h.Value
		case "from":
			result.From = h.Value
		case "to":
			result.To = h.Value
		case "date":
			result.Date = h.Value
		}
	}
	if format == "full" {
		result.Text = plainParts(raw.Payload)
		if result.Text == "" {
			result.Text = htmlParts(raw.Payload)
		}
		if result.Text == "" {
			result.Text = raw.Snippet + "\n[No plain-text body. Open this message in Gmail to read its full formatted contents.]"
		}
	}
	return result, nil
}

// Default to recent mail. An explicit date range can reach older messages.
func recentGmailQuery(query string) string {
	query = strings.TrimSpace(query)
	for _, term := range []string{"after:", "before:", "older:", "newer:", "older_than:", "newer_than:"} {
		if strings.Contains(strings.ToLower(query), term) {
			return query
		}
	}
	if query == "" {
		return "newer_than:30d"
	}
	return "(" + query + ") newer_than:30d"
}

// SearchGmail reads summaries on demand; nothing is imported or marked read.
func SearchGmail(ctx context.Context, owner, query, page string, limit int) (GmailPage, error) {
	out := GmailPage{Messages: []GmailMessage{}}
	query = recentGmailQuery(query)
	q := url.Values{"q": {query}, "maxResults": {fmt.Sprint(boundedLimit(limit))}}
	if page != "" {
		q.Set("pageToken", page)
	}
	var list struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		Next string `json:"nextPageToken"`
	}
	if err := readJSON(ctx, owner, GmailScope, "https://gmail.googleapis.com/gmail/v1/users/me/messages?"+q.Encode(), &list); err != nil {
		return out, err
	}
	out.NextPage = list.Next
	for _, m := range list.Messages {
		entry, err := gmailMessage(ctx, owner, m.ID, "metadata")
		if err != nil {
			return out, err
		}
		out.Messages = append(out.Messages, entry)
	}
	return out, nil
}
func ReadGmail(ctx context.Context, owner, id string) (GmailMessage, error) {
	return gmailMessage(ctx, owner, id, "full")
}

type DriveFile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	Modified string `json:"modifiedTime"`
	URL      string `json:"webViewLink"`
}
type DrivePage struct {
	Files    []DriveFile `json:"files"`
	NextPage string      `json:"nextPageToken,omitempty"`
}
type DriveContent struct {
	File DriveFile `json:"file"`
	Text string    `json:"text"`
}

// SearchDrive searches the connected account, without copying its files locally.
func SearchDrive(ctx context.Context, owner, query, page string, limit int) (DrivePage, error) {
	out := DrivePage{Files: []DriveFile{}}
	filter := "trashed = false"
	if query = strings.TrimSpace(query); query != "" {
		escaped := strings.NewReplacer("\\", "\\\\", "'", "\\'").Replace(query)
		filter += " and fullText contains '" + escaped + "'"
	}
	q := url.Values{"q": {filter}, "pageSize": {fmt.Sprint(boundedLimit(limit))}, "orderBy": {"modifiedTime desc"}, "fields": {"nextPageToken,files(id,name,mimeType,modifiedTime,webViewLink)"}}
	if page != "" {
		q.Set("pageToken", page)
	}
	err := readJSON(ctx, owner, DriveScope, "https://www.googleapis.com/drive/v3/files?"+q.Encode(), &out)
	return out, err
}

// ReadDrive supports text files and text exports of Docs, Sheets and Slides.
func ReadDrive(ctx context.Context, owner, id string) (DriveContent, error) {
	var out DriveContent
	if !validID(id) {
		return out, fmt.Errorf("invalid Drive file id")
	}
	endpoint := "https://www.googleapis.com/drive/v3/files/" + id
	if err := readJSON(ctx, owner, DriveScope, endpoint+"?fields=id,name,mimeType,modifiedTime,webViewLink", &out.File); err != nil {
		return out, err
	}
	switch out.File.MimeType {
	case "application/vnd.google-apps.document", "application/vnd.google-apps.presentation":
		endpoint += "/export?mimeType=text%2Fplain"
	case "application/vnd.google-apps.spreadsheet":
		endpoint += "/export?mimeType=text%2Fcsv"
	default:
		if !strings.HasPrefix(out.File.MimeType, "text/") && out.File.MimeType != "application/json" {
			return out, fmt.Errorf("this file cannot be read as text; open it in Drive")
		}
		endpoint += "?alt=media"
	}
	b, err := readAPI(ctx, owner, DriveScope, endpoint)
	out.Text = string(b)
	return out, err
}
