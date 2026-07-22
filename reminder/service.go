package reminder

import (
	"context"
	"strings"
)

// Server is the go-micro service handler for the daily Islamic reminder. Its
// methods are exposed as RPC endpoints and, through the agent and gateways, as
// AI tools.
type Server struct{}

// TodayRequest takes no input.
type TodayRequest struct{}

// TodayResponse is today's reminder as model-ready text.
type TodayResponse struct {
	Reminder string `json:"reminder" description:"Today's Islamic reminder: Quran verse with reference, a hadith, and a short reflection"`
}

// Today returns today's Islamic reminder — a Quran verse with its surah
// reference, a hadith, and a short reflection.
// @example {}
func (Server) Today(_ context.Context, _ *TodayRequest, rsp *TodayResponse) error {
	d := GetDailyReminderData()
	if d == nil {
		rsp.Reminder = "No reminder is available right now."
		return nil
	}
	var b strings.Builder
	if d.Name != "" {
		b.WriteString(d.Name + "\n")
	}
	if d.Verse != "" {
		b.WriteString(d.Verse + "\n")
	}
	if d.Hadith != "" {
		b.WriteString("\nHadith: " + d.Hadith + "\n")
	}
	if d.Message != "" {
		b.WriteString("\nReflection: " + d.Message)
	}
	rsp.Reminder = strings.TrimSpace(b.String())
	return nil
}
