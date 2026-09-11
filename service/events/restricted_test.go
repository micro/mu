package events

import (
	"context"
	"mu/internal/service"
	"testing"
	"time"
)

func TestRestrictedCallerCannotScheduleAnUnrestrictedAgent(t *testing.T) {
	ctx := service.WithRestrictedCaller(service.WithAccount(context.Background(), "restricted-owner"))
	var rsp CreateResponse
	req := &CreateRequest{Title: "Appointment", When: time.Now().Add(time.Hour).Format(time.RFC3339), Prompt: "Read private mail"}
	if err := (Server{}).Create(ctx, req, &rsp); err == nil {
		t.Fatal("restricted caller scheduled work")
	}
	req.Prompt = ""
	if err := (Server{}).Create(ctx, req, &rsp); err != nil {
		t.Fatal(err)
	}
	defer Cancel("restricted-owner", rsp.Item.ID)
	prompt := "Read private mail"
	var updated UpdateResponse
	if err := (Server{}).Update(ctx, &UpdateRequest{ID: rsp.Item.ID, Prompt: &prompt}, &updated); err == nil {
		t.Fatal("restricted caller upgraded reminder to agent work")
	}
}
