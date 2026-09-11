package events

import (
	"testing"
	"time"
)

func TestOnlyUnchangedCalendarIdentityIsCollapsed(t *testing.T) {
	e := &Event{ID: "local", Title: "Webinar", When: day(12, 0)}
	copy := External{UID: "local@mu", Title: e.Title, Start: e.When, End: e.When.Add(e.Length())}
	if got := withoutLocalCopies([]*Event{e}, []External{copy}); len(got) != 0 {
		t.Fatal("unchanged imported occurrence duplicated")
	}
	for _, mutate := range []func(*External){
		func(x *External) { x.UID = "" },
		func(x *External) { x.UID = "other@mu" },
		func(x *External) { x.Start = x.Start.Add(time.Hour) },
		func(x *External) { x.End = x.End.Add(time.Hour) },
		func(x *External) { x.Title = "Edited webinar" },
		func(x *External) { x.Location = "Meeting room" },
		func(x *External) { x.AllDay = true },
	} {
		x := copy
		mutate(&x)
		if got := withoutLocalCopies([]*Event{e}, []External{x}); len(got) != 1 {
			t.Fatalf("ambiguous or changed occurrence hidden: %+v", x)
		}
	}
	if got := withoutLocalCopies(nil, []External{copy}); len(got) != 1 {
		t.Fatal("another owner's event was used to collapse a copy")
	}
}

func TestExternalCopiesUseOnlyTheCallersUpcomingEvents(t *testing.T) {
	when := time.Now().Add(time.Hour).Truncate(time.Second)
	e, err := Create("copy-owner", "Webinar", when, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { Cancel(e.Owner, e.ID) })
	old := ExternalEntries
	t.Cleanup(func() { ExternalEntries = old })
	ExternalEntries = func(string, time.Time, time.Time, int) []External {
		return []External{{UID: e.ID + "@mu", Title: e.Title, Start: e.When, End: e.When.Add(e.Length())}}
	}
	if got := ExternalEvents(e.Owner, when, when.Add(time.Hour), 0); len(got) != 0 {
		t.Fatal("owner sees duplicate imported occurrence")
	}
	if got := ExternalEvents("different-owner", when, when.Add(time.Hour), 0); len(got) != 1 {
		t.Fatal("another owner's local event affected the result")
	}
	if got := Overview(e.Owner, PreviewLimit); len(got) != 0 {
		t.Fatal("overview duplicated imported occurrence")
	}
	if err := Cancel(e.Owner, e.ID); err != nil {
		t.Fatal(err)
	}
	if got := CachedOverview(e.Owner); len(got) != 1 {
		t.Fatal("cached merge hid the independently surviving calendar copy")
	}
}
