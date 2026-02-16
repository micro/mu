package chat

import (
	"testing"
	"time"
)

// TestNewXMPPServer tests server initialization
func TestNewXMPPServer(t *testing.T) {
	server := NewXMPPServer("test.example.com", "5222")

	if server == nil {
		t.Fatal("Expected server to be created, got nil")
	}

	if server.Domain != "test.example.com" {
		t.Errorf("Expected domain 'test.example.com', got '%s'", server.Domain)
	}

	if server.Port != "5222" {
		t.Errorf("Expected port '5222', got '%s'", server.Port)
	}

	if server.sessions == nil {
		t.Error("Expected sessions map to be initialized")
	}

	if len(server.sessions) != 0 {
		t.Errorf("Expected 0 sessions initially, got %d", len(server.sessions))
	}
}

// TestGenerateStreamID tests stream ID generation
func TestGenerateStreamID(t *testing.T) {
	id1 := generateStreamID()
	if id1 == "" {
		t.Error("Expected non-empty stream ID")
	}

	// Wait a bit to ensure different timestamp
	time.Sleep(1 * time.Millisecond)

	id2 := generateStreamID()
	if id2 == "" {
		t.Error("Expected non-empty stream ID")
	}

	if id1 == id2 {
		t.Error("Expected different stream IDs for different calls")
	}
}

// TestGetXMPPStatus tests status retrieval
func TestGetXMPPStatus(t *testing.T) {
	// Test when server is nil (not started)
	status := GetXMPPStatus()

	if status["enabled"] != false {
		t.Error("Expected enabled to be false when server is nil")
	}

	// Create a server instance
	xmppServer = NewXMPPServer("test.example.com", "5222")

	status = GetXMPPStatus()

	if status["enabled"] != true {
		t.Error("Expected enabled to be true when server exists")
	}

	if status["domain"] != "test.example.com" {
		t.Errorf("Expected domain 'test.example.com', got '%v'", status["domain"])
	}

	if status["port"] != "5222" {
		t.Errorf("Expected port '5222', got '%v'", status["port"])
	}

	if status["sessions"] != 0 {
		t.Errorf("Expected 0 sessions, got '%v'", status["sessions"])
	}

	// Clean up
	xmppServer = nil
}

// TestXMPPServerStop tests graceful shutdown
func TestXMPPServerStop(t *testing.T) {
	server := NewXMPPServer("test.example.com", "5222")

	// Stop should not error even if listener is nil
	err := server.Stop()
	if err != nil {
		t.Errorf("Expected no error on stop with nil listener, got %v", err)
	}

	// Check that context is cancelled
	select {
	case <-server.ctx.Done():
		// Context cancelled as expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected context to be cancelled after Stop()")
	}
}
