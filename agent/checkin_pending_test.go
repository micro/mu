package agent

import (
	"mu/internal/thread"
	"mu/service/mail"
	"testing"
	"time"
)

func TestPendingCheckinLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	owner := "pending-checkin-owner"
	loc := time.FixedZone("local", 3600)
	now := time.Date(2026, 9, 30, 0, 30, 0, 0, loc)
	add := func(account, key string, at time.Time) *thread.Thread {
		th := thread.OpenAt(account, "mail", key, at)
		thread.Add(thread.Message{Account: account, Thread: th.ID, Role: thread.RolePerson, From: "agent@" + mail.ConfiguredDomain(), To: account + "+checkin@test", Text: "What is your focus?", At: at})
		return th
	}
	old := add(owner, "yesterday", now.Add(-24*time.Hour))
	thread.MarkSeen(owner, old.ID)
	add("foreign-owner", "foreign", now.Add(-time.Minute))
	if PendingCheckin(owner, now) != nil {
		t.Fatal("old or foreign check-in became an action")
	}
	today := add(owner, "today", now.Add(-20*time.Minute))
	thread.MarkSeen(owner, today.ID)
	if got := PendingCheckin(owner, now); got == nil || got.ID != today.ID {
		t.Fatal("reading a check-in completed it, or UTC hid today's check-in")
	}
	live, err := consolidateCheckin(owner, today)
	if err != nil || live == nil {
		t.Fatalf("consolidate: %v", err)
	}
	if got := PendingCheckin(owner, now); got == nil || got.ID != live.ID {
		t.Fatal("consolidation lost pending check-in")
	}
	thread.Add(thread.Message{Account: owner, Thread: live.ID, Role: thread.RolePerson, Text: "Finish the proposal", At: now.Add(-10 * time.Minute)})
	thread.Add(thread.Message{Account: owner, Thread: live.ID, Role: thread.RoleAgent, Text: "Let's do it", At: now.Add(-9 * time.Minute)})
	if PendingCheckin(owner, now) != nil {
		t.Fatal("answered check-in remained actionable")
	}
	newer := add(owner, "replacement", now.Add(-5*time.Minute))
	if got := PendingCheckin(owner, now); got == nil || got.ID != newer.ID {
		t.Fatal("new check-in did not replace answered one")
	}
	if PendingCheckin(owner, now.AddDate(0, 0, 1)) != nil {
		t.Fatal("yesterday's check-in survived rollover")
	}
}
