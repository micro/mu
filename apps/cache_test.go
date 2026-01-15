package apps

import (
	"testing"
	"time"
)

func TestLLMCaching(t *testing.T) {
	// Test basic cache operations
	systemPrompt := "Test system prompt"
	userPrompt := "Create a simple counter app"
	testCode := "<html><body>Test app</body></html>"
	
	// Initially should have no cache hit
	if code, ok := getCachedLLMResponse(systemPrompt, userPrompt); ok {
		t.Errorf("Expected cache miss, got hit with code: %s", code)
	}
	
	// Cache a response
	cacheLLMResponse(systemPrompt, userPrompt, testCode)
	
	// Should get cache hit now
	code, ok := getCachedLLMResponse(systemPrompt, userPrompt)
	if !ok {
		t.Errorf("Expected cache hit, got miss")
	}
	if code != testCode {
		t.Errorf("Expected code %q, got %q", testCode, code)
	}
	
	// Different prompt should miss
	if code, ok := getCachedLLMResponse(systemPrompt, "Different prompt"); ok {
		t.Errorf("Expected cache miss for different prompt, got hit with code: %s", code)
	}
}

func TestLLMCacheExpiration(t *testing.T) {
	// Save original TTL and restore after test
	originalTTL := llmCacheTTL
	defer func() { llmCacheTTL = originalTTL }()
	
	// Set very short TTL for testing
	llmCacheTTL = 100 * time.Millisecond
	
	systemPrompt := "Test system"
	userPrompt := "Test user prompt"
	testCode := "<html><body>Expiring test</body></html>"
	
	// Cache a response
	cacheLLMResponse(systemPrompt, userPrompt, testCode)
	
	// Should hit immediately
	if _, ok := getCachedLLMResponse(systemPrompt, userPrompt); !ok {
		t.Errorf("Expected cache hit immediately after caching")
	}
	
	// Wait for expiration (with some buffer for timing variations)
	time.Sleep(200 * time.Millisecond)
	
	// Should miss after expiration
	if code, ok := getCachedLLMResponse(systemPrompt, userPrompt); ok {
		t.Errorf("Expected cache miss after expiration, got hit with code: %s", code)
	}
}

func TestHashPrompt(t *testing.T) {
	// Same inputs should produce same hash
	hash1 := hashPrompt("system", "user")
	hash2 := hashPrompt("system", "user")
	if hash1 != hash2 {
		t.Errorf("Same inputs should produce same hash: %s vs %s", hash1, hash2)
	}
	
	// Different inputs should produce different hashes
	hash3 := hashPrompt("different", "user")
	if hash1 == hash3 {
		t.Errorf("Different inputs should produce different hashes")
	}
	
	// Hash should be deterministic
	hash4 := hashPrompt("system", "user")
	if hash1 != hash4 {
		t.Errorf("Hash should be deterministic: %s vs %s", hash1, hash4)
	}
}

func TestClearExpiredLLMCache(t *testing.T) {
	// Save original TTL and restore after test
	originalTTL := llmCacheTTL
	defer func() { llmCacheTTL = originalTTL }()
	
	// Set short TTL
	llmCacheTTL = 100 * time.Millisecond
	
	// Add some cache entries
	cacheLLMResponse("sys1", "user1", "code1")
	cacheLLMResponse("sys2", "user2", "code2")
	
	// Wait for expiration (with buffer for timing variations)
	time.Sleep(200 * time.Millisecond)
	
	// Add a fresh entry
	cacheLLMResponse("sys3", "user3", "code3")
	
	// Clear expired entries
	clearExpiredLLMCache()
	
	// Old entries should be gone
	if _, ok := getCachedLLMResponse("sys1", "user1"); ok {
		t.Errorf("Expired entry should have been cleared")
	}
	if _, ok := getCachedLLMResponse("sys2", "user2"); ok {
		t.Errorf("Expired entry should have been cleared")
	}
	
	// Fresh entry should still be there
	if _, ok := getCachedLLMResponse("sys3", "user3"); !ok {
		t.Errorf("Fresh entry should not have been cleared")
	}
}
