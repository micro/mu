package agent

import (
	"strings"
	"testing"
)

func TestNoToolsSystemDoesNotDemandTools(t *testing.T) {
	s := nativeSystem(QueryOpts{NoTools: true, System: "Write from supplied sources."})
	if strings.Contains(s, "Use the available tools") || strings.Contains(s, "use the events Create") {
		t.Fatal(s)
	}
	if !strings.Contains(s, "Source material was collected") {
		t.Fatal(s)
	}
}
