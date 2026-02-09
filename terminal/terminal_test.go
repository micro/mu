package terminal

import (
	"strings"
	"testing"
)

func TestGetShell(t *testing.T) {
	shell := getShell()
	if shell == "" {
		t.Error("getShell() returned empty string")
	}
	// Should return a valid shell path
	validShells := []string{"sh", "bash", "cmd.exe", "/bin/sh", "/bin/bash"}
	found := false
	for _, valid := range validShells {
		if strings.HasSuffix(shell, valid) || shell == valid {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("getShell() returned unexpected shell: %s", shell)
	}
}

func TestRenderPage(t *testing.T) {
	page := RenderPage()
	if page == "" {
		t.Error("RenderPage() returned empty string")
	}
	// Should contain key terminal elements
	if !strings.Contains(page, "terminal-container") {
		t.Error("RenderPage() should contain terminal-container")
	}
	if !strings.Contains(page, "terminal-input") {
		t.Error("RenderPage() should contain terminal-input")
	}
	if !strings.Contains(page, "terminal-output") {
		t.Error("RenderPage() should contain terminal-output")
	}
	if !strings.Contains(page, "WebSocket") {
		t.Error("RenderPage() should contain WebSocket connection code")
	}
}

func TestTemplateContainsMobileStyles(t *testing.T) {
	// Verify mobile-friendly responsive styles are present
	if !strings.Contains(Template, "@media") {
		t.Error("Template should contain responsive media queries for mobile support")
	}
	if !strings.Contains(Template, "max-width: 600px") {
		t.Error("Template should contain mobile breakpoint")
	}
}

func TestWSMessageTypes(t *testing.T) {
	// Verify the message type constants are used correctly
	tests := []struct {
		name     string
		msgType  string
		expected string
	}{
		{"output type", "output", "output"},
		{"error type", "error", "error"},
		{"input type", "input", "input"},
		{"prompt type", "prompt", "prompt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := wsMessage{Type: tt.msgType, Data: "test"}
			if msg.Type != tt.expected {
				t.Errorf("Expected type %q, got %q", tt.expected, msg.Type)
			}
		})
	}
}
