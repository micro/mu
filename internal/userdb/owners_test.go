package userdb

import "testing"

func TestOwnersMatchingOnlyInterruptedWork(t *testing.T) {
	for _, v := range []struct{ owner, status string }{{"one", "doing"}, {"two", "done"}, {"one", "doing"}, {"three", "doing"}} {
		if _, err := Create("recovery_fixture", v.owner, "tasks", map[string]interface{}{"status": v.status, "assignee": "agent"}, false); err != nil {
			t.Fatal(err)
		}
	}
	owners, err := OwnersMatching("recovery_fixture", "tasks", map[string]interface{}{"status": "doing", "assignee": "agent"})
	if err != nil || len(owners) != 2 || owners[0] != "one" || owners[1] != "three" {
		t.Fatal(owners, err)
	}
}
