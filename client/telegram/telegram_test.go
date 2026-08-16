package telegram

import (
	"os"
	"strings"
	"testing"
)

func TestSanitizeAccountID(t *testing.T) {
	tests := []struct {
		name       string
		telegramID string
		username   string
		want       string
	}{
		{
			name:       "lowercases and removes unsupported characters",
			telegramID: "123456789",
			username:   "Mu.User-Name!",
			want:       "muusername",
		},
		{
			name:       "pads short usernames with telegram suffix",
			telegramID: "123456789",
			username:   "Al",
			want:       "al6789",
		},
		{
			name:       "caps account IDs at auth limit",
			telegramID: "123456789",
			username:   "averyveryverylongtelegramusername",
			want:       "averyveryverylongtelegra",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeAccountID(tt.telegramID, tt.username); got != tt.want {
				t.Fatalf("sanitizeAccountID(%q, %q) = %q, want %q", tt.telegramID, tt.username, got, tt.want)
			}
		})
	}
}

func TestUniqueAccountIDPreservesSuffixForLongBase(t *testing.T) {
	base := "averyveryverylongtelegra"
	existing := map[string]bool{base: true}

	got := uniqueAccountID(base, func(id string) bool { return existing[id] })
	want := "averyveryverylongtelegr1"
	if got != want {
		t.Fatalf("uniqueAccountID() = %q, want %q", got, want)
	}
	if len(got) > 24 {
		t.Fatalf("uniqueAccountID() length = %d, want <= 24", len(got))
	}
}

func TestUniqueAccountIDExhaustion(t *testing.T) {
	base := "taken"
	got := uniqueAccountID(base, func(id string) bool { return true })
	if got != "" {
		t.Fatalf("uniqueAccountID() = %q, want empty string when exhausted", got)
	}
}

// A client keeps no conversation history of its own.
//
// It kept a map keyed by Telegram id: in memory, lost on restart, invisible on
// /agents, and one of three such maps that had all drifted apart. History
// belongs to the agent now, where it is persisted, shared, and the same shape
// whichever client a message came through — see agent.Ask.
func TestTheClientKeepsNoHistoryOfItsOwn(t *testing.T) {
	src, err := os.ReadFile("telegram.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"histories", "getHistory", "addHistory"} {
		if strings.Contains(string(src), gone) {
			t.Errorf("telegram still has %s — conversation history is the agent's, and "+
				"a second copy here is one that restarts empty and nothing else can read", gone)
		}
	}
	if !strings.Contains(string(src), "agent.Ask(") {
		t.Error("telegram no longer goes through agent.Ask, so it has its own surround again")
	}
}
