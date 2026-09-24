package service

import "time"

// SourceRequest and SourceResponse are the read contract for a stored message.
// They expose no transport secrets or attachment bytes.
type SourceRequest struct {
	ID string `json:"id" required:"true" description:"Stored message id"`
}
type SourceResponse struct {
	Item *SourceMessage `json:"item"`
}
type SourceMessage struct {
	Facts        map[string]interface{} `json:"facts,omitempty"` // Transport facts recorded at arrival; consumers own response policy.
	InReplyTo    string                 `json:"in_reply_to,omitempty"`
	References   string                 `json:"references,omitempty"`
	HTML         string                 `json:"html,omitempty"`
	ID           string                 `json:"id"`
	Ref          string                 `json:"ref"`
	Conversation string                 `json:"conversation"`
	Subject      string                 `json:"subject,omitempty"`
	Text         string                 `json:"text"`
	From         string                 `json:"from"`
	To           string                 `json:"to,omitempty"`
	Channel      string                 `json:"channel,omitempty"`
	Direction    string                 `json:"direction,omitempty"`
	At           time.Time              `json:"at"`
}

type SourcesRequest struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}
type SourcesResponse struct {
	IDs  []string `json:"ids"`
	Next int      `json:"next,omitempty"`
}
