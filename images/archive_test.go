package images

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func resetArchive() {
	archiveMu.Lock()
	archive = nil
	archiveMu.Unlock()
}

func TestArchiveKeepsHistoryNewestFirst(t *testing.T) {
	resetArchive()
	defer resetArchive()

	archiveDaily(Daily{Date: "2026-07-01", URL: "u1", Theme: "nature"})
	archiveDaily(Daily{Date: "2026-07-02", URL: "u2", Theme: "space"})
	archiveDaily(Daily{Date: "2026-07-03", URL: "u3", Theme: "ocean"})

	got := Archive(0)
	if len(got) != 3 {
		t.Fatalf("archive length = %d, want 3", len(got))
	}
	for i, want := range []string{"2026-07-03", "2026-07-02", "2026-07-01"} {
		if got[i].Date != want {
			t.Errorf("entry %d = %s, want %s", i, got[i].Date, want)
		}
	}
}

// Regenerating the same day replaces that day rather than adding a duplicate.
func TestArchiveReplacesSameDay(t *testing.T) {
	resetArchive()
	defer resetArchive()

	archiveDaily(Daily{Date: "2026-07-01", URL: "first"})
	archiveDaily(Daily{Date: "2026-07-01", URL: "second"})

	got := Archive(0)
	if len(got) != 1 {
		t.Fatalf("archive length = %d, want 1", len(got))
	}
	if got[0].URL != "second" {
		t.Errorf("URL = %q, want %q", got[0].URL, "second")
	}
}

func TestArchiveCapsAndReportsDropped(t *testing.T) {
	resetArchive()
	defer resetArchive()

	for i := 0; i < archiveMax; i++ {
		archiveDaily(Daily{Date: dateFor(i), URL: "u", File: "f"})
	}
	if n := len(Archive(0)); n != archiveMax {
		t.Fatalf("archive length = %d, want %d", n, archiveMax)
	}

	dropped := archiveDaily(Daily{Date: dateFor(archiveMax), URL: "u", File: "old"})
	if len(dropped) != 1 {
		t.Fatalf("dropped = %d entries, want 1", len(dropped))
	}
	if n := len(Archive(0)); n != archiveMax {
		t.Errorf("archive length = %d after trim, want %d", n, archiveMax)
	}
}

// dateFor builds distinct consecutive dates for the cap test.
func dateFor(i int) string {
	return time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).
		AddDate(0, 0, i).Format("2006-01-02")
}

func TestPastDailiesExcludesToday(t *testing.T) {
	resetArchive()
	defer resetArchive()

	archiveDaily(Daily{Date: "2026-07-01", URL: "u1"})
	archiveDaily(Daily{Date: "2026-07-02", URL: "u2"})

	past := pastDailies("2026-07-02", 0)
	if len(past) != 1 || past[0].Date != "2026-07-01" {
		t.Errorf("pastDailies = %+v, want only 2026-07-01", past)
	}
}

// The date segment names a storage key, so anything that is not a date must be
// rejected rather than reaching the store.
func TestDailyImageHandlerRejectsBadPaths(t *testing.T) {
	resetArchive()
	defer resetArchive()
	archiveDaily(Daily{Date: "2026-07-01", URL: "u", File: "images/daily/2026-07-01.png"})

	bad := []string{
		"/images/daily/../../accounts",
		"/images/daily/..%2f..%2faccounts",
		"/images/daily/accounts.json",
		"/images/daily/",
		"/images/daily/9999-99-99",
		"/images/daily/2026-07-01.png",
	}
	for _, p := range bad {
		rec := httptest.NewRecorder()
		DailyImageHandler(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s returned %d, want 404", p, rec.Code)
		}
	}
}

func TestDailyImageHandlerMissingFileIs404(t *testing.T) {
	resetArchive()
	defer resetArchive()
	// Archived, but no stored bytes — must not 500.
	archiveDaily(Daily{Date: "2026-07-01", URL: "https://provider/x.png"})

	rec := httptest.NewRecorder()
	DailyImageHandler(rec, httptest.NewRequest(http.MethodGet, "/images/daily/2026-07-01", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestDisplayURLPrefersLocalCopy(t *testing.T) {
	local := Daily{Date: "2026-07-01", URL: "https://provider/x.png", File: "images/daily/2026-07-01.png"}
	if got := local.displayURL(); got != "/images/daily/2026-07-01" {
		t.Errorf("displayURL = %q, want local path", got)
	}

	remote := Daily{Date: "2026-07-01", URL: "https://provider/x.png"}
	if got := remote.displayURL(); got != "https://provider/x.png" {
		t.Errorf("displayURL = %q, want provider URL fallback", got)
	}
}

func TestImageExt(t *testing.T) {
	cases := []struct{ ct, url, want string }{
		{"image/png", "", ".png"},
		{"image/jpeg; charset=binary", "", ".jpg"},
		{"image/webp", "", ".webp"},
		{"", "https://x/y/z.jpg?sig=abc", ".jpg"},
		{"", "https://x/y/z.webp", ".webp"},
		{"application/octet-stream", "https://x/y/z", ".png"},
	}
	for _, c := range cases {
		if got := imageExt(c.ct, c.url); got != c.want {
			t.Errorf("imageExt(%q, %q) = %q, want %q", c.ct, c.url, got, c.want)
		}
	}
}
