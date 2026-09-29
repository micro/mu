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
