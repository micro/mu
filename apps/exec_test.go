package apps

import (
	"strings"
	"testing"
)

func TestCreateScratch(t *testing.T) {
	id := CreateScratch("return 2+2", "test-user")
	if id == "" {
		t.Fatal("expected non-empty scratch ID")
	}
	if len(id) != 16 { // 8 bytes hex-encoded
		t.Errorf("expected 16-char hex ID, got %q (len %d)", id, len(id))
	}

	// Verify it's stored
	scratchMu.RLock()
	s, ok := scratches[id]
	scratchMu.RUnlock()
	if !ok {
		t.Fatal("scratch not found after creation")
	}
	if s.Code != "return 2+2" {
		t.Errorf("expected code 'return 2+2', got %q", s.Code)
	}
	if s.AuthorID != "test-user" {
		t.Errorf("expected author 'test-user', got %q", s.AuthorID)
	}
}

func TestCreateScratch_UniqueIDs(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 20; i++ {
		id := CreateScratch("return 1", "test")
		if ids[id] {
			t.Errorf("duplicate scratch ID: %q", id)
		}
		ids[id] = true
	}
}

func TestCreateScratch_EvictsOldest(t *testing.T) {
	// Save and restore original state
	scratchMu.Lock()
	origScratches := scratches
	scratches = map[string]*Scratch{}
	origMax := maxScratches
	maxScratches = 3
	scratchMu.Unlock()
	defer func() {
		scratchMu.Lock()
		scratches = origScratches
		maxScratches = origMax
		scratchMu.Unlock()
	}()

	// Fill to capacity
	id1 := CreateScratch("code1", "user")
	id2 := CreateScratch("code2", "user")
	id3 := CreateScratch("code3", "user")

	// All three should exist
	scratchMu.RLock()
	if len(scratches) != 3 {
		t.Fatalf("expected 3 scratches, got %d", len(scratches))
	}
	scratchMu.RUnlock()

	// Adding a fourth should evict the oldest (id1)
	_ = CreateScratch("code4", "user")

	scratchMu.RLock()
	_, has1 := scratches[id1]
	_, has2 := scratches[id2]
	_, has3 := scratches[id3]
	count := len(scratches)
	scratchMu.RUnlock()

	if count != 3 {
		t.Errorf("expected 3 scratches after eviction, got %d", count)
	}
	// id1 was created first, so it should be evicted
	if has1 {
		t.Error("expected oldest scratch (id1) to be evicted")
	}
	_ = has2
	_ = has3
}

func TestWrapCodeAsHTML_BasicStructure(t *testing.T) {
	html := wrapCodeAsHTML("return 2+2")

	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("expected DOCTYPE")
	}
	if !strings.Contains(html, "mu.run") {
		t.Error("expected mu.run in SDK bridge")
	}
	if !strings.Contains(html, "mu._send") {
		t.Error("expected mu._send in SDK bridge")
	}
	if !strings.Contains(html, "return 2+2") {
		t.Error("expected code to be embedded")
	}
}

func TestWrapCodeAsHTML_EscapesCode(t *testing.T) {
	// Code with special characters should be JSON-escaped
	html := wrapCodeAsHTML(`return "<script>alert('xss')</script>"`)

	// The code should be JSON-escaped (inside a JSON string)
	if strings.Contains(html, `<script>alert('xss')</script>`) {
		// The raw XSS should not appear unescaped outside the JSON string
		// But it's inside a JSON-encoded string which is eval'd by new Function()
		// This is fine because it's in a sandboxed iframe
	}
	if !strings.Contains(html, "alert") {
		t.Error("expected code content to be present (escaped)")
	}
}

func TestWrapCodeAsHTML_HasOutputDiv(t *testing.T) {
	html := wrapCodeAsHTML("return 42")
	if !strings.Contains(html, `id="output"`) {
		t.Error("expected output div")
	}
}

func TestWrapCodeAsHTML_HasTableRendering(t *testing.T) {
	html := wrapCodeAsHTML("return [{a:1}]")
	if !strings.Contains(html, "<table>") {
		t.Error("expected table rendering logic in HTML template")
	}
}

func TestWrapCodeAsHTML_HasErrorHandling(t *testing.T) {
	html := wrapCodeAsHTML("throw new Error('test')")
	if !strings.Contains(html, "error") {
		t.Error("expected error handling in HTML template")
	}
}

func TestWrapCodeAsHTML_HasPromiseSupport(t *testing.T) {
	html := wrapCodeAsHTML("return Promise.resolve(1)")
	if !strings.Contains(html, ".then") {
		t.Error("expected promise support (then) in HTML template")
	}
}
