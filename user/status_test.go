package user

import (
	"fmt"
	"testing"
	"time"
)

func TestContainsMention(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"@micro hello", true},
		{"hey @micro can you help", true},
		{"prefix @micro", true},
		{"@micro", true},
		{"@micro!", true},
		{"@micro, please", true},
		{"visit @microwave", false},
		{"@microsoft is a company", false},
		{"email me@micro.xyz", false},
		{"no mention at all", false},
		{"", false},
		{"@micro @micro twice", true},
	}
	for _, tc := range cases {
		got := containsMention(tc.text, MicroMention)
		if got != tc.want {
			t.Errorf("containsMention(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestStatusStream_ChronologicalOrder(t *testing.T) {
	profileMutex.Lock()
	saved := profiles
	profiles = map[string]*Profile{}
	profileMutex.Unlock()
	t.Cleanup(func() {
		profileMutex.Lock()
		profiles = saved
		profileMutex.Unlock()
	})

	now := time.Now()
	profileMutex.Lock()
	profiles["alice"] = &Profile{
		UserID:    "alice",
		Status:    "latest",
		UpdatedAt: now,
		History: []StatusHistory{
			{Status: "oldest", SetAt: now.Add(-5 * time.Minute)},
			{Status: "middle", SetAt: now.Add(-2 * time.Minute)},
		},
	}
	profiles["bob"] = &Profile{
		UserID:    "bob",
		Status:    "bob now",
		UpdatedAt: now.Add(-1 * time.Minute),
		History: []StatusHistory{
			{Status: "bob old", SetAt: now.Add(-3 * time.Minute)},
		},
	}
	profileMutex.Unlock()

	stream := StatusStream(100, "")

	// Expected order (newest first):
	//   alice "latest" (now)
	//   bob "bob now" (-1m)
	//   alice "middle" (-2m)
	//   bob "bob old" (-3m)
	//   alice "oldest" (-5m)
	wantOrder := []string{"latest", "bob now", "middle", "bob old", "oldest"}
	if len(stream) != len(wantOrder) {
		t.Fatalf("got %d entries, want %d: %+v", len(stream), len(wantOrder), stream)
	}
	for i, w := range wantOrder {
		if stream[i].Status != w {
			t.Errorf("stream[%d].Status = %q, want %q", i, stream[i].Status, w)
		}
	}
}

func TestStatusStream_PerUserCapPreventsFlood(t *testing.T) {
	profileMutex.Lock()
	saved := profiles
	profiles = map[string]*Profile{}
	profileMutex.Unlock()
	t.Cleanup(func() {
		profileMutex.Lock()
		profiles = saved
		profileMutex.Unlock()
	})

	now := time.Now()

	// Alice is an active chatter with 20 recent messages.
	var aliceHistory []StatusHistory
	for i := 0; i < 20; i++ {
		aliceHistory = append(aliceHistory, StatusHistory{
			Status: fmt.Sprintf("alice %d", i),
			SetAt:  now.Add(-time.Duration(i+1) * time.Minute),
		})
	}
	profileMutex.Lock()
	profiles["alice"] = &Profile{
		UserID:    "alice",
		Status:    "alice now",
		UpdatedAt: now,
		History:   aliceHistory,
	}
	// Bob posted once half an hour ago — should still appear even
	// though Alice has 20 messages in between.
	profiles["bob"] = &Profile{
		UserID:    "bob",
		Status:    "bob says hi",
		UpdatedAt: now.Add(-30 * time.Minute),
	}
	profileMutex.Unlock()

	// Cap: 10 total, 3 per user. Alice should contribute at most 3.
	stream := StatusStreamCapped(10, 3, "")

	aliceCount := 0
	bobCount := 0
	for _, e := range stream {
		if e.UserID == "alice" {
			aliceCount++
		}
		if e.UserID == "bob" {
			bobCount++
		}
	}
	if aliceCount > 3 {
		t.Errorf("alice contributed %d entries, want at most 3", aliceCount)
	}
	if bobCount != 1 {
		t.Errorf("bob contributed %d entries, want 1", bobCount)
	}
	// Alice's 3 entries should be her 3 most recent, not random.
	var aliceStatuses []string
	for _, e := range stream {
		if e.UserID == "alice" {
			aliceStatuses = append(aliceStatuses, e.Status)
		}
	}
	want := []string{"alice now", "alice 0", "alice 1"}
	if len(aliceStatuses) != len(want) {
		t.Fatalf("got %d alice entries, want %d", len(aliceStatuses), len(want))
	}
	for i, w := range want {
		if aliceStatuses[i] != w {
			t.Errorf("aliceStatuses[%d] = %q, want %q", i, aliceStatuses[i], w)
		}
	}
}

func TestStatusStream_RespectsMax(t *testing.T) {
	profileMutex.Lock()
	saved := profiles
	profiles = map[string]*Profile{}
	profileMutex.Unlock()
	t.Cleanup(func() {
		profileMutex.Lock()
		profiles = saved
		profileMutex.Unlock()
	})

	now := time.Now()
	var history []StatusHistory
	for i := 0; i < 50; i++ {
		history = append(history, StatusHistory{
			Status: fmt.Sprintf("old %d", i),
			SetAt:  now.Add(-time.Duration(i+1) * time.Minute),
		})
	}
	profileMutex.Lock()
	profiles["alice"] = &Profile{
		UserID:    "alice",
		Status:    "current",
		UpdatedAt: now,
		History:   history,
	}
	profileMutex.Unlock()

	stream := StatusStream(10, "")
	if len(stream) != 10 {
		t.Errorf("got %d, want 10", len(stream))
	}
	if stream[0].Status != "current" {
		t.Errorf("newest should be 'current', got %q", stream[0].Status)
	}
}

func TestUpdateProfile_AlwaysAppendsHistory(t *testing.T) {
	profileMutex.Lock()
	saved := profiles
	profiles = map[string]*Profile{}
	profileMutex.Unlock()
	t.Cleanup(func() {
		profileMutex.Lock()
		profiles = saved
		profileMutex.Unlock()
	})

	// First status — no history yet.
	p := &Profile{UserID: "alice", Status: "hello"}
	if err := UpdateProfile(p); err != nil {
		t.Fatalf("first update: %v", err)
	}

	// Second status — previous should be pushed.
	p2 := &Profile{UserID: "alice", Status: "world"}
	if err := UpdateProfile(p2); err != nil {
		t.Fatalf("second update: %v", err)
	}
	if len(p2.History) != 1 || p2.History[0].Status != "hello" {
		t.Errorf("after second update, history = %+v, want one entry 'hello'", p2.History)
	}

	// Third status — even when the text repeats, the previous is pushed.
	p3 := &Profile{UserID: "alice", Status: "world"}
	if err := UpdateProfile(p3); err != nil {
		t.Fatalf("third update: %v", err)
	}
	if len(p3.History) != 2 || p3.History[0].Status != "world" || p3.History[1].Status != "hello" {
		t.Errorf("after third update, history = %+v, want ['world', 'hello']", p3.History)
	}
}
