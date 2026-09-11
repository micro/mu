package events

import (
	"context"
	"fmt"
	"mu/internal/data"
	"mu/internal/service"
	"strings"
	"time"
)

type UpdateRequest struct {
	ID      string  `json:"id" required:"true" description:"Event id from events_list"`
	Title   *string `json:"title,omitempty" description:"New title; omitted keeps the existing title"`
	When    *string `json:"when,omitempty" description:"New future time as RFC3339 with timezone offset"`
	Note    *string `json:"note,omitempty" description:"New note; empty clears it"`
	Minutes *int    `json:"minutes,omitempty" description:"Duration in minutes, 1 to 10080"`
	Repeat  *string `json:"repeat,omitempty" description:"hourly, daily, weekly, monthly; empty makes it one-off"`
	Prompt  *string `json:"prompt,omitempty" description:"New standing instruction; empty clears it"`
}
type UpdateResponse struct {
	Item *Event `json:"item"`
	Text string `json:"text"`
}

// Update edits the owner's event under the scheduler lock and persists before
// returning. An already-fired event needs a new future time to be reactivated.
func (Server) Update(ctx context.Context, req *UpdateRequest, rsp *UpdateResponse) error {
	owner := service.AccountFrom(ctx)
	if owner == "" {
		return fmt.Errorf("sign in to change events")
	}
	mu.Lock()
	defer mu.Unlock()
	old := events[strings.TrimSpace(req.ID)]
	if old == nil || old.Owner != owner {
		return fmt.Errorf("event not found")
	}
	if service.RestrictedCaller(ctx) && (old.Prompt != "" || (req.Prompt != nil && strings.TrimSpace(*req.Prompt) != "")) {
		return fmt.Errorf("a restricted caller cannot change background agent work")
	}
	next := *old
	if req.Title != nil {
		next.Title = strings.TrimSpace(*req.Title)
	}
	if next.Title == "" || len(next.Title) > 1000 {
		return fmt.Errorf("title must be 1 to 1000 bytes")
	}
	if req.Note != nil {
		next.Note = strings.TrimSpace(*req.Note)
	}
	if req.Prompt != nil {
		next.Prompt = strings.TrimSpace(*req.Prompt)
	}
	if len(next.Note) > 16000 || len(next.Prompt) > 16000 {
		return fmt.Errorf("note and prompt must not exceed 16000 bytes")
	}
	if req.Minutes != nil {
		if *req.Minutes < 1 || *req.Minutes > 10080 {
			return fmt.Errorf("minutes must be between 1 and 10080")
		}
		next.Minutes = *req.Minutes
	}
	if req.Repeat != nil {
		repeat := strings.TrimSpace(*req.Repeat)
		next.Repeat = ParseRepeat(repeat)
		if repeat != "" && next.Repeat == "" {
			return fmt.Errorf("repeat must be hourly, daily, weekly or monthly")
		}
	}
	if req.When != nil {
		when, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.When))
		if err != nil || !when.After(time.Now()) {
			return fmt.Errorf("when must be a future RFC3339 time with timezone")
		}
		next.When, next.Fired, next.FiredAt = when.UTC(), false, time.Time{}
	} else if next.Fired || !next.When.After(time.Now()) {
		return fmt.Errorf("supply a new future time for an event already due or fired")
	}
	next.Sequence++
	events[next.ID] = &next
	list := make([]*Event, 0, len(events))
	for _, e := range events {
		list = append(list, e)
	}
	if err := data.SaveJSON(storeKey, list); err != nil {
		events[next.ID] = old
		return err
	}
	cp := next
	rsp.Item = &cp
	rsp.Text = "Updated: " + Describe(&next)
	if OnCreate != nil {
		invite := next
		go OnCreate(&invite)
	}
	return nil
}
