package moderate

import "testing"

func TestProfanityIsRejectedWithoutAModel(t *testing.T) {
	v, err := classify("", "Bring it on you little fuckers")
	if err != nil || v != "HARMFUL" {
		t.Fatalf("verdict=%q err=%v", v, err)
	}
	if Approved("", "Bring it on you little fuckers") {
		t.Fatal("approved explicit abuse")
	}
}

func TestNoModelCannotApproveImportedContent(t *testing.T) {
	if Configured() {
		t.Skip("test requires no configured provider")
	}
	if Approved("", "An ordinary news update.") {
		t.Fatal("approved without moderation")
	}
}
