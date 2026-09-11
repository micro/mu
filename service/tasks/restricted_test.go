package tasks

import (
	"context"
	"mu/internal/service"
	"testing"
	"time"
)

func TestRestrictedCallerCannotBorrowBackgroundAgent(t *testing.T) {
	setupTasks(t)
	ctx := service.WithRestrictedCaller(service.WithAccount(context.Background(), "someone"))
	var rsp TaskResponse
	if err := (Server{}).Create(ctx, &CreateRequest{Title: "Read private mail", Assignee: Agent}, &rsp); err == nil {
		t.Fatal("restricted caller started work")
	}
	if len(List("someone", "")) != 0 {
		t.Fatal("refused task persisted")
	}
	if err := (Server{}).Create(ctx, &CreateRequest{Title: "Personal reminder", Assignee: Me}, &rsp); err != nil {
		t.Fatal(err)
	}
	task, err := Create("someone", "Existing work", "", Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if err := (Server{}).Update(ctx, &UpdateRequest{ID: task.ID, Detail: "Read private mail"}, &rsp); err == nil {
		t.Fatal("restricted caller changed agent instructions")
	}
	got, _ := Get("someone", task.ID)
	if got.Detail != "" {
		t.Fatal("refused update persisted")
	}
}
