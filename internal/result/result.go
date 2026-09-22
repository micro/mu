// Package result describes durable objects returned in a conversation.
// These are data, never model-authored HTML or executable code.
package result

type Point struct{ Lat, Lon float64 }
type Item struct {
	Body    string   `json:"body,omitempty"`
	Kind    string   `json:"kind"`
	ID      string   `json:"id,omitempty"`
	URL     string   `json:"url,omitempty"`
	Title   string   `json:"title,omitempty"`
	Summary string   `json:"summary,omitempty"`
	Shape   []Point  `json:"shape,omitempty"`
	Steps   []string `json:"steps,omitempty"`
}
