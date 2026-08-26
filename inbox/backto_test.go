package inbox

import (
	"strings"
	"testing"

	"mu/internal/thread"
)

// A conversation that happened in a room says where the room is.
//
// The note told somebody to answer where it arrived and then did not say
// where that was. agent/chat opens the thread with the room id as its key, so
// the address was always knowable and simply was not a link.
func TestAChatThreadLinksBackToItsRoom(t *testing.T) {
	got := backTo(&thread.Thread{Client: thread.ChatClient, Key: "chat_micro"})
	if !strings.Contains(got, `href="/chat?id=chat_micro"`) {
		t.Errorf("no way back to the room: %q", got)
	}
}

// And one that arrived by something with no URL says nothing, rather than
// linking somewhere that cannot be opened.
func TestAThreadWithNowhereToGoBackToSaysNothing(t *testing.T) {
	for _, tt := range []*thread.Thread{
		{Client: "sms", Key: "+447700900000"},
		{Client: thread.ChatClient, Key: ""},
		nil,
	} {
		if got := backTo(tt); got != "" {
			t.Errorf("%+v produced a link it cannot honour: %q", tt, got)
		}
	}
}
