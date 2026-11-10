package mucp

import "testing"

func TestPagesBindCallerAndOperation(t *testing.T) {
	_, token, err := Page([]int{1, 2, 3}, 1, "", "alice/inbox")
	if err != nil || token == "" {
		t.Fatal(err)
	}
	for _, scope := range []string{"bob/inbox", "alice/contacts"} {
		if _, _, err := Page([]int{1, 2, 3}, 1, token, scope); err == nil {
			t.Fatal("accepted foreign cursor")
		}
	}
	page, _, err := Page([]int{1, 2, 3}, 1, token, "alice/inbox")
	if err != nil || len(page) != 1 || page[0] != 2 {
		t.Fatal(page, err)
	}
	if _, _, err := Page([]int{1, 2, 3}, 101, "", "alice/inbox"); err == nil {
		t.Fatal("unbounded page")
	}
}
