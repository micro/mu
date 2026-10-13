package digest

import (
	"encoding/json"
	"mu/internal/auth"
	"mu/service/blog"
	"testing"
	"time"
)

func TestDailyDigestSurvivesRestartAndHourlyChecks(t *testing.T) {
	now := time.Date(2026, 10, 13, 20, 0, 0, 0, time.UTC)
	original := []*blog.Post{{ID: "today", AuthorID: auth.MicroID, Tags: "digest", CreatedAt: now.Add(-12 * time.Hour)}}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var restored []*blog.Post
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	for _, posts := range [][]*blog.Post{original, restored} {
		if p := todayFrom(posts, now); p == nil || p.ID != "today" {
			t.Fatal("same-day digest must prevent another publication, including after restart")
		}
		if todayFrom(posts, now.AddDate(0, 0, 1)) != nil {
			t.Fatal("yesterday must not block today's digest")
		}
	}
}

func TestDailyDigestIgnoresOtherPosts(t *testing.T) {
	now := time.Now()
	posts := []*blog.Post{
		{AuthorID: auth.MicroID, Tags: "digest", Private: true, CreatedAt: now},
		{AuthorID: "reader", Tags: "digest", CreatedAt: now},
		{AuthorID: auth.MicroID, Tags: "opinion,world", CreatedAt: now},
	}
	if todayFrom(posts, now) != nil {
		t.Fatal("unrelated post blocked daily digest")
	}
}
