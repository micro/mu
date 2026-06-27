package flag

import (
	"testing"
)

func resetFlags(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	mutex.Lock()
	flags = make(map[string]*FlaggedItem)
	deleters = make(map[string]ContentDeleter)
	analyzer = nil
	mutex.Unlock()
}

func TestAdd_FirstFlag(t *testing.T) {
	resetFlags(t)

	count, alreadyFlagged, err := Add("post", "123", "alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
	if alreadyFlagged {
		t.Error("should not be already flagged on first flag")
	}
}

func TestAdd_DuplicateFlag(t *testing.T) {
	resetFlags(t)

	Add("post", "123", "alice")
	count, alreadyFlagged, err := Add("post", "123", "alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count still 1, got %d", count)
	}
	if !alreadyFlagged {
		t.Error("should report already flagged for same user")
	}
}

func TestAdd_ThreeFlags_HidesContent(t *testing.T) {
	resetFlags(t)

	Add("post", "456", "alice")
	Add("post", "456", "bob")
	count, _, _ := Add("post", "456", "charlie")

	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}

	if !IsHidden("post", "456") {
		t.Error("content should be hidden after 3 flags")
	}
}

func TestGetFlags(t *testing.T) {
	resetFlags(t)

	count, flagged := GetFlags("post", "nonexistent")
	if count != 0 {
		t.Errorf("expected 0 for nonexistent, got %d", count)
	}
	if flagged {
		t.Error("nonexistent should not be flagged")
	}

	Add("post", "789", "alice")
	count, flagged = GetFlags("post", "789")
	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}
	if flagged {
		t.Error("should not be flagged with only 1 flag")
	}
}

func TestGetCount(t *testing.T) {
	resetFlags(t)

	if GetCount("post", "nonexistent") != 0 {
		t.Error("expected 0 for nonexistent")
	}

	Add("post", "abc", "alice")
	Add("post", "abc", "bob")
	if GetCount("post", "abc") != 2 {
		t.Errorf("expected 2, got %d", GetCount("post", "abc"))
	}
}

func TestGetItem(t *testing.T) {
	resetFlags(t)

	if GetItem("post", "nonexistent") != nil {
		t.Error("expected nil for nonexistent")
	}

	Add("post", "xyz", "alice")
	item := GetItem("post", "xyz")
	if item == nil {
		t.Fatal("expected item")
	}
	if item.ContentType != "post" {
		t.Errorf("expected content_type 'post', got %q", item.ContentType)
	}
	if item.ContentID != "xyz" {
		t.Errorf("expected content_id 'xyz', got %q", item.ContentID)
	}
}

func TestGetAll(t *testing.T) {
	resetFlags(t)

	Add("post", "1", "alice")
	Add("thread", "2", "bob")

	items := GetAll()
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestApprove(t *testing.T) {
	resetFlags(t)

	Add("post", "approve-me", "alice")
	Add("post", "approve-me", "bob")
	Add("post", "approve-me", "charlie")

	if !IsHidden("post", "approve-me") {
		t.Error("should be hidden before approve")
	}

	Approve("post", "approve-me")

	if IsHidden("post", "approve-me") {
		t.Error("should not be hidden after approve")
	}
	if GetCount("post", "approve-me") != 0 {
		t.Error("flags should be cleared after approve")
	}
}

func TestIsHidden(t *testing.T) {
	resetFlags(t)

	if IsHidden("post", "none") {
		t.Error("nonexistent should not be hidden")
	}

	Add("post", "hide-me", "a")
	Add("post", "hide-me", "b")
	Add("post", "hide-me", "c")

	if !IsHidden("post", "hide-me") {
		t.Error("should be hidden after 3 flags")
	}
}

func TestAdminFlag(t *testing.T) {
	resetFlags(t)

	AdminFlag("post", "admin-flag", "admin_user")

	item := GetItem("post", "admin-flag")
	if item == nil {
		t.Fatal("expected item after admin flag")
	}
	if item.FlagCount != 3 {
		t.Errorf("expected count 3 (admin immediate), got %d", item.FlagCount)
	}
	if !item.Flagged {
		t.Error("should be flagged after admin flag")
	}
	if !IsHidden("post", "admin-flag") {
		t.Error("should be hidden after admin flag")
	}
}

func TestAdminFlag_SameAdminDoesNotDuplicateFlagger(t *testing.T) {
	resetFlags(t)

	AdminFlag("post", "admin-flag", "admin_user")
	AdminFlag("post", "admin-flag", "admin_user")

	item := GetItem("post", "admin-flag")
	if item == nil {
		t.Fatal("expected item after admin flag")
	}
	if got, want := len(item.FlaggedBy), 1; got != want {
		t.Fatalf("expected %d flagger, got %d: %v", want, got, item.FlaggedBy)
	}
	if got, want := item.FlaggedBy[0], "admin_user (admin)"; got != want {
		t.Errorf("expected admin flagger %q, got %q", want, got)
	}
}

func TestAdminFlag_ExistingItem(t *testing.T) {
	resetFlags(t)

	Add("post", "existing", "alice")
	AdminFlag("post", "existing", "admin_user")

	item := GetItem("post", "existing")
	if item.FlagCount != 3 {
		t.Errorf("expected 3, got %d", item.FlagCount)
	}
	if !item.Flagged {
		t.Error("should be flagged")
	}
}

func TestContains(t *testing.T) {
	if !contains([]string{"a", "b", "c"}, "b") {
		t.Error("expected true for existing item")
	}
	if contains([]string{"a", "b", "c"}, "d") {
		t.Error("expected false for missing item")
	}
	if contains(nil, "a") {
		t.Error("expected false for nil slice")
	}
}

func TestDelete(t *testing.T) {
	resetFlags(t)

	Add("post", "delete-me", "alice")
	Delete("post", "delete-me")

	if GetItem("post", "delete-me") != nil {
		t.Error("item should be removed after delete")
	}
}

func TestRegisterDeleter(t *testing.T) {
	_, ok := GetDeleter("unknown")
	if ok {
		t.Error("expected false for unregistered type")
	}
}

type stubAnalyzer struct {
	response string
	err      error
}

func (s stubAnalyzer) Analyze(prompt, question string) (string, error) {
	return s.response, s.err
}

func TestCheckContent_AutoFlagsSuspiciousContent(t *testing.T) {
	resetFlags(t)
	SetAnalyzer(stubAnalyzer{response: " spam "})

	CheckContent("post", "spam-post", "Cheap stuff", "Buy now")

	item := GetItem("post", "spam-post")
	if item == nil {
		t.Fatal("expected suspicious content to be flagged")
	}
	if !item.Flagged {
		t.Error("expected suspicious content to be hidden immediately")
	}
	if got, want := item.FlagCount, 3; got != want {
		t.Errorf("expected flag count %d, got %d", want, got)
	}
	if got, want := item.FlaggedBy[0], "system:spam (admin)"; got != want {
		t.Errorf("expected flagger %q, got %q", want, got)
	}
}

func TestCheckContent_OKDoesNotFlag(t *testing.T) {
	resetFlags(t)
	SetAnalyzer(stubAnalyzer{response: "OK"})

	CheckContent("post", "normal-post", "Daily update", "Working on tests")

	if item := GetItem("post", "normal-post"); item != nil {
		t.Fatalf("expected OK content to remain unflagged, got %#v", item)
	}
}

func TestCheckContent_NilAnalyzer(t *testing.T) {
	resetFlags(t)
	SetAnalyzer(nil)
	// Should not panic
	CheckContent("post", "1", "title", "content")
}
