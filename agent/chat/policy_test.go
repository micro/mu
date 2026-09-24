package chat

import "testing"

func TestRoomResponsePolicy(t *testing.T) {
	for _, tc := range []struct {
		room, text   string
		n            float64
		direct, want bool
	}{
		{"chat_group", "hello", 2, false, false},
		{"chat_alone", "hello", 1, false, true},
		{"chat_group", "@Micro help", 2, false, true},
		{"dm_opaque", "hello", 2, true, true},
		{"news_item", "explain this", 2, false, true},
	} {
		if got := shouldAnswer(map[string]interface{}{"room": tc.room, "text": tc.text, "participants": tc.n, "direct": tc.direct}); got != tc.want {
			t.Errorf("%+v: %v", tc, got)
		}
	}
}
