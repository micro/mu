package home

import (
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestStatusInlineEditor(t *testing.T) {
	if statusForm(httptest.NewRequest("GET", "/home", nil), "") != "" {
		t.Fatal("guest status editor")
	}
	markup := statusForm(httptest.NewRequest("GET", "/home", nil), "status-test")
	for _, unwanted := range []string{"<details", "<form", ">Save<", ">Clear<", ">Edit<"} {
		if strings.Contains(markup, unwanted) {
			t.Fatalf("unexpected control %s", unwanted)
		}
	}
	if !strings.Contains(markup, "data-status-input hidden") {
		t.Fatal("editor must start hidden")
	}
}

func TestStatusEditorLifecycle(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	if out, err := exec.Command(node, "testdata/status.cjs").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

func TestStatusSaveRequiresSession(t *testing.T) {
	r := httptest.NewRequest("POST", "/home", strings.NewReader("action=status&status=Hello"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	statusHandler(w, r)
	if w.Code != 401 {
		t.Fatalf("got %d, want unauthorized", w.Code)
	}
}
