package agent

import (
	"strings"
	"testing"
)

func TestReadingSourceValidation(t *testing.T) {
	if usableReadingSource("Access denied") {
		t.Fatal("accepted block page")
	}
	if usableReadingSource(strings.Repeat("Checking your browser ", 100)) {
		t.Fatal("accepted challenge")
	}
	if !usableReadingSource(strings.Repeat("A meaningful passage from the source. ", 30)) {
		t.Fatal("rejected text")
	}
}
func TestNoToolsSystemDoesNotDemandTools(t *testing.T) {
	s := nativeSystem(QueryOpts{NoTools: true, System: "Write from supplied sources."})
	if strings.Contains(s, "Use the available tools") || strings.Contains(s, "use the events Create") {
		t.Fatal(s)
	}
	if !strings.Contains(s, "Source material was collected") {
		t.Fatal(s)
	}
}

func TestReadingRejectsCatalogueAndUnsupportedOutput(t *testing.T) {
	catalogue := "Recent Titles\nPopular Titles\nAuthors of the Week\n" + strings.Repeat("Sirat Ibn Hisham Al-Mizan Books Articles Magazines ", 40)
	if usableReadingSource(catalogue) {
		t.Fatal("accepted catalogue as source prose")
	}
	for _, raw := range []string{`{"substantive":false,"reading":"A shelf reveals a community."}`, `{"reading":"Unassessed essay"}`, `{"substantive":true,"reading":""}`, "An unvalidated essay"} {
		if _, err := parseEveningReading(raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if text, err := parseEveningReading("```json\n{\"substantive\":true,\"reading\":\"# Patience\\nSupported reading.\"}\n```"); err != nil || text != "# Patience\nSupported reading." {
		t.Fatalf("%q %v", text, err)
	}
}
