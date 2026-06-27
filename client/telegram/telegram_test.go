package telegram

import (
	"testing"

	"mu/agent"
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

func TestGetHistoryReturnsCopy(t *testing.T) {
	telegramID := "history-copy-test"
	historyMu.Lock()
	histories[telegramID] = []agent.QueryMessage{{Role: "user", Text: "original"}}
	historyMu.Unlock()
	defer func() {
		historyMu.Lock()
		delete(histories, telegramID)
		historyMu.Unlock()
	}()

	got := getHistory(telegramID)
	got[0].Text = "mutated"

	again := getHistory(telegramID)
	if again[0].Text != "original" {
		t.Fatalf("getHistory returned mutable backing storage; got %q", again[0].Text)
	}
}
