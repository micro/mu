package thread

import (
	"fmt"
	"testing"
)

func TestMessageWindowIsBoundedAndOwned(t *testing.T) {
	const owner = "window_test"
	th := Open(owner, WebClient, "window")
	t.Cleanup(func() { Forget(owner) })
	for i := 0; i < 65; i++ {
		Add(Message{Account: owner, Thread: th.ID, Text: fmt.Sprint(i)})
	}
	first, more := MessageWindow(owner, th.ID, 0, 25)
	if len(first) != 25 || first[0].Text != "40" || !more {
		t.Fatal("wrong latest window")
	}
	second, more := MessageWindow(owner, th.ID, 25, 25)
	if len(second) != 25 || second[0].Text != "15" || second[24].Text != "39" || !more {
		t.Fatal("overlap or skipped messages")
	}
	last, more := MessageWindow(owner, th.ID, 50, 25)
	if len(last) != 15 || last[0].Text != "0" || more {
		t.Fatal("wrong final window")
	}
	if other, _ := MessageWindow("stranger", th.ID, 0, 100); len(other) != 0 {
		t.Fatal("cross-owner messages")
	}
}
