package thread

import (
	"mu/internal/data"
	"strings"
	"testing"
)

func TestAttachmentIsAnOwnedPersistedReference(t *testing.T) {
	const owner = "attachment_owner"
	th := Open(owner, "web", "attachment-test")
	defer Forget(owner)
	SetAttachment(owner, th.ID, "saved:opaque-id")
	SetAttachment("someone_else", th.ID, "saved:wrong-id")
	if Attachment(owner, th.ID) != "saved:opaque-id" || Attachment("someone_else", th.ID) != "" {
		t.Fatal("attachment crossed owners")
	}
	SetAttachment(owner, th.ID, strings.Repeat("a", 257))
	if Attachment(owner, th.ID) != "saved:opaque-id" {
		t.Fatal("accepted unbounded reference")
	}
	Flush()
	var stored struct {
		Threads []Thread `json:"threads"`
	}
	if err := data.LoadJSON("threads.json", &stored); err != nil {
		t.Fatal(err)
	}
	for _, got := range stored.Threads {
		if got.ID == th.ID {
			if got.Attachment != "saved:opaque-id" {
				t.Fatal("reference lost on persistence")
			}
			return
		}
	}
	t.Fatal("conversation was not persisted")
}
