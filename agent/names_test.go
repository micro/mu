package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/userdb"
)

func TestReservedNamesAcrossOwnersAndRename(t *testing.T) {
	const label = "Global name test"
	a, _, err := CreateAgent("names_owner_a", label, Hosted, "private instruction", "public description", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := CreateAgent("names_owner_b", label, Hosted, "another instruction", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	first, second := Slug(a), Slug(b)
	if first == second {
		t.Fatal("duplicate global names")
	}
	if id, ok := BySlug(a.Owner, first); !ok || id != a.ID {
		t.Fatal("owner cannot resolve")
	}
	if _, ok := BySlug(b.Owner, first); ok {
		t.Fatal("name resolved for wrong account")
	}
	edited, err := UpdateAgent(a.Owner, a.ID, "Changed display label", a.Prompt, a.Description, nil)
	if err != nil {
		t.Fatal(err)
	}
	if Slug(edited) != first {
		t.Fatal("display rename moved URL")
	}
	if err := RemoveAgent(a.Owner, a.ID); err != nil {
		t.Fatal(err)
	}
	c, _, err := CreateAgent("names_owner_c", label, Hosted, "prompt", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if Slug(c) == first {
		t.Fatal("deleted address was reclaimed")
	}
	if _, ok := BySlug(c.Owner, first); ok {
		t.Fatal("deleted address resolved")
	}
	builtin, _, err := CreateAgent("names_owner_d", "Micro", Hosted, "prompt", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if Slug(builtin) == "micro" {
		t.Fatal("built-in name taken")
	}
}

func TestNameMigrationPreservesIDsAndExistingSuffixes(t *testing.T) {
	resetNameCache()
	t.Cleanup(resetNameCache)
	var agents []*Agent
	for i, tag := range []string{"migrationname", "migrationname", "migrationname2"} {
		a := &Agent{Owner: []string{"migration_a", "migration_b", "migration_c"}[i], Name: tag, Tag: tag, Prompt: "private"}
		rec, err := userdb.Create(ns, a.Owner, collection, fields(a), false)
		if err != nil {
			t.Fatal(err)
		}
		a.ID = rec.ID
		agents = append(agents, a)
	}
	if Slug(agents[0]) != "migrationname" || Slug(agents[1]) != "migrationname3" || Slug(agents[2]) != "migrationname2" {
		t.Fatal("migration did not preserve existing unique names")
	}
	for _, a := range agents {
		before := Slug(a)
		restored := For(a.Owner, a.ID)
		if restored == nil || restored.Tag != a.Tag || Slug(restored) != before {
			t.Fatal("migration changed identity or mail tag")
		}
	}
}

func TestConcurrentNameReservations(t *testing.T) {
	var wg sync.WaitGroup
	results := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, _, err := CreateAgent("concurrent_names", "Concurrent reserved", Hosted, "prompt", "", nil, false)
			if err != nil {
				t.Error(err)
				return
			}
			results <- Slug(a)
		}()
	}
	wg.Wait()
	close(results)
	seen := map[string]bool{}
	for name := range results {
		if seen[name] {
			t.Fatal("duplicate reservation")
		}
		seen[name] = true
	}
	if len(seen) != 8 {
		t.Fatal("missing reservations")
	}
}

func TestPublicProfileOnlyExposesPublishedMetadata(t *testing.T) {
	const owner = "profile_owner"
	a, _, err := CreateAgent(owner, "Profile test", Hosted, "SECRET INSTRUCTIONS", "Public description <script>alert(1)</script>", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	path := "/agent/" + Slug(a)
	get := func(token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if token != "" {
			r.AddCookie(&http.Cookie{Name: "session", Value: token})
		}
		w := httptest.NewRecorder()
		ConsoleHandler(w, r)
		return w
	}
	if w := get(""); w.Code != 404 {
		t.Fatalf("private profile: %d", w.Code)
	}
	if err := SetProfile("other", a.ID, true); err == nil {
		t.Fatal("nonowner published agent")
	}
	if err := SetProfile(owner, a.ID, true); err != nil {
		t.Fatal(err)
	}
	for _, visitor := range []string{"", "profile_other"} {
		token := ""
		if visitor != "" {
			auth.SetAccountForTest(&auth.Account{ID: visitor})
			defer auth.RemoveAccountForTest(visitor)
			session, err := auth.CreateSession(visitor)
			if err != nil {
				t.Fatal(err)
			}
			token = session.Token
		}
		w := get(token)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "Profile test") || !strings.Contains(w.Body.String(), "Public description") {
			t.Fatalf("profile: %d %s", w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "SECRET INSTRUCTIONS") || strings.Contains(w.Body.String(), "<script>alert(1)</script>") {
			t.Fatal("private or unescaped contents exposed")
		}
	}
	// Public visibility does not authorise the HTTP execution endpoint.
	auth.SetAccountForTest(&auth.Account{ID: "profile_api_other", Admin: true})
	defer auth.RemoveAccountForTest("profile_api_other")
	_, token, err := auth.CreateToken("profile_api_other", "profile", []string{"read", "write", "api:agent"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", path, strings.NewReader(`{"prompt":"hello"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	APIHandler(response, request)
	if response.Code != 404 {
		t.Fatalf("foreign execution: %d %s", response.Code, response.Body.String())
	}
	// The record itself stays private even when its safe projection is published.
	if _, err := userdb.Get(ns, "", collection, a.ID); err == nil {
		t.Fatal("published raw record")
	}
	if _, ok := BySlug("profile_other", Slug(a)); ok {
		t.Fatal("publication granted execution")
	}
	if err := SetProfile(owner, a.ID, false); err != nil {
		t.Fatal(err)
	}
	if w := get(""); w.Code != 404 {
		t.Fatal("unpublished profile still visible")
	}
}

func resetNameCache() {
	namesMu.Lock()
	defer namesMu.Unlock()
	namesCache = nil
	namesByAgent = nil
}

func TestColdRosterMutationsReserveOldNames(t *testing.T) {
	for _, action := range []string{"rename", "delete"} {
		t.Run(action, func(t *testing.T) {
			tag := "coldmutation" + action
			a := &Agent{Owner: "cold_" + action, Name: tag, Tag: tag, Prompt: "instruction"}
			rec, err := userdb.Create(ns, a.Owner, collection, fields(a), false)
			if err != nil {
				t.Fatal(err)
			}
			a.ID = rec.ID
			resetNameCache()
			t.Cleanup(resetNameCache)
			if action == "rename" {
				if _, err := UpdateAgent(a.Owner, a.ID, "new label", a.Prompt, "", nil); err != nil {
					t.Fatal(err)
				}
			} else if err := RemoveAgent(a.Owner, a.ID); err != nil {
				t.Fatal(err)
			}
			// Reload from disk too: the reservation must survive a process restart.
			resetNameCache()
			reserved, err := names()
			if err != nil {
				t.Fatal(err)
			}
			if reserved[tag] != (nameOwner{a.Owner, a.ID}) {
				t.Fatal("original address was not reserved")
			}
			other, _, err := CreateAgent("other_"+action, tag, Hosted, "prompt", "", nil, false)
			if err != nil {
				t.Fatal(err)
			}
			if Slug(other) == tag {
				t.Fatal("old address was reclaimed")
			}
		})
	}
}

func TestWarmNamesDoNotRepeatMigration(t *testing.T) {
	resetNameCache()
	t.Cleanup(resetNameCache)
	if _, err := names(); err != nil {
		t.Fatal(err)
	}
	// A raw legacy record is deliberately invisible to a warm index. This
	// detects accidental roster rescanning on Slug/namedAgent calls.
	a := &Agent{Owner: "warm_index", Name: "warmlegacy", Tag: "warmlegacy", Prompt: "instruction"}
	rec, err := userdb.Create(ns, a.Owner, collection, fields(a), false)
	if err != nil {
		t.Fatal(err)
	}
	a.ID = rec.ID
	if got, err := namedAgent(a.Tag); err != nil || got != nil {
		t.Fatal("warm lookup reran migration")
	}
	if Slug(a) != a.ID {
		t.Fatal("warm reverse lookup reran migration")
	}
	// The supported creation path explicitly updates the cache and persistence.
	created, _, err := CreateAgent("warm_index", "warmcreated", Hosted, "prompt", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	slug := Slug(created)
	if slug == created.ID {
		t.Fatal("creation did not update index")
	}
	resetNameCache()
	if Slug(created) != slug || Slug(a) != a.Tag {
		t.Fatal("restart did not restore/migrate names")
	}
}
