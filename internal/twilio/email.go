package twilio

// Email, on the same account as the texts.
//
// Twilio has a native email API — POST comms.twilio.com/v1/Emails, HTTP basic
// with the credentials this package already holds — so sending mail from a
// verified domain needs no second provider and no second key. That is worth
// stating plainly because the first attempt at this reached for SendGrid, which
// Twilio owns but which authenticates separately with its own SG key: an extra
// credential to create, store, rotate and mask, for a capability the account
// already had.
//
// This package's job is unchanged: the credentials, the call, and no opinion
// about what may be sent. Who may send, to whom, how often and under whose name
// belongs to service/email, exactly as it belongs to service/sms for a text.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// emailEndpoint is Twilio's mail send resource.
const emailEndpoint = "https://comms.twilio.com/v1/Emails"

// Email is one message to hand over.
//
// Plain and HTML both, because a message carrying only one of them is scored
// worse by every filter that looks at it.
type Email struct {
	From     string
	FromName string
	// ReplyTo is where answers should go, and Twilio cannot carry it.
	//
	// There is no reply_to field on the Mail Send resource, and setting the
	// header is refused outright: "The header 'Reply-To' is restricted and
	// cannot be overridden." So a message sent this way is answered at its From
	// address and nowhere else, and the consequence is a DNS one rather than a
	// code one — the sending domain needs an MX record, or replies to it bounce
	// and the sender never learns they were answered.
	//
	// Kept on the struct because the SMTP path can set it and does, and because
	// a caller should not have to know which carrier is underneath to describe
	// what it wants. Twilio drops it; see Send.
	ReplyTo string
	To      string
	ToName  string
	Subject string
	Text    string
	HTML    string
}

// EmailConfigured reports whether this instance can send email through Twilio.
func EmailConfigured() bool {
	user, pass := Credentials()
	return user != "" && pass != ""
}

// SendEmail hands one message over and returns Twilio's operation id.
//
// The API answers 202 with an operationId and a URL to ask about it later —
// there is no message id, because nothing has been delivered yet at the moment
// it replies. The id is the handle for asking what became of it.
func SendEmail(m Email) (string, error) {
	if !EmailConfigured() {
		return "", fmt.Errorf("no Twilio credentials, so email cannot be sent")
	}
	if strings.TrimSpace(m.From) == "" || strings.TrimSpace(m.To) == "" {
		return "", fmt.Errorf("a sender and a recipient are both required")
	}

	from := map[string]string{"address": m.From}
	if m.FromName != "" {
		from["name"] = m.FromName
	}
	to := map[string]string{"address": m.To}
	if m.ToName != "" {
		to["name"] = m.ToName
	}
	content := map[string]any{"subject": m.Subject}
	if m.Text != "" {
		content["text"] = m.Text
	}
	if m.HTML != "" {
		content["html"] = m.HTML
	}
	// Deliberately not sent. See Email.ReplyTo: Twilio refuses the header with a
	// 400, so passing it through would fail every message rather than degrade
	// one.

	raw, err := json.Marshal(map[string]any{
		"from":    from,
		"to":      []map[string]string{to},
		"content": content,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", emailEndpoint, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	user, pass := Credentials()
	req.SetBasicAuth(user, pass)
	req.Header.Set("Content-Type", "application/json")

	rsp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("could not reach Twilio: %w", err)
	}
	defer rsp.Body.Close() //nolint:errcheck

	body, _ := io.ReadAll(io.LimitReader(rsp.Body, 16<<10))
	if rsp.StatusCode >= 300 {
		return "", fmt.Errorf("Twilio refused it: %s", emailError(body, rsp.Status))
	}
	var ok struct {
		OperationID string `json:"operationId"`
	}
	json.Unmarshal(body, &ok) //nolint:errcheck
	return ok.OperationID, nil
}

// emailError pulls Twilio's own explanation out of a refusal, so an operator
// reads "the from domain is not verified" rather than "400".
func emailError(body []byte, status string) string {
	var e struct {
		// The shape a refusal actually arrives in — an array. Reading only the
		// top level put the whole JSON blob in front of an operator, with the
		// one sentence that explained it buried in the middle.
		Errors []struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"errors"`
		Message string `json:"message"`
		Detail  string `json:"detail"`
		Title   string `json:"title"`
	}
	if json.Unmarshal(body, &e) == nil {
		var parts []string
		for _, x := range e.Errors {
			if strings.TrimSpace(x.Message) != "" {
				parts = append(parts, x.Message)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "; ")
		}
		for _, s := range []string{e.Message, e.Detail, e.Title} {
			if strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	if len(body) > 0 {
		return status + ": " + strings.TrimSpace(string(body))
	}
	return status
}
