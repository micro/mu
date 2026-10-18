package agent

import (
	"context"
	gmai "go-micro.dev/v6/model"
	"mu/internal/group"
	"testing"
)

func TestGroupToolsRecheckMemberAndScheduleRights(t *testing.T) {
	g, _ := group.Create("tool-owner", "Tools", false)
	defer group.Change(g.ID, g.Owner, "delete", "", "")
	_ = group.Change(g.ID, g.Owner, "invite", "tool-member", "")
	_ = group.Change(g.ID, "tool-member", "accept", "", "")
	calls := 0
	next := func(context.Context, gmai.ToolCall) gmai.ToolResult { calls++; return gmai.ToolResult{Content: "ok"} }
	guarded := groupToolGuard(group.Scope(g.ID), "tool-member")(next)
	if got := guarded(context.Background(), gmai.ToolCall{Name: "events.Server.Create"}); got.Refused == "" || calls != 0 {
		t.Fatal("member changed schedules through agent")
	}
	guarded(context.Background(), gmai.ToolCall{Name: "events.Server.List"})
	if calls != 1 {
		t.Fatal("member cannot read group events")
	}
	_ = group.Change(g.ID, g.Owner, "remove", "tool-member", "")
	if got := guarded(context.Background(), gmai.ToolCall{Name: "news.Server.List"}); got.Refused == "" || calls != 1 {
		t.Fatal("revoked member continued tool execution")
	}
}
