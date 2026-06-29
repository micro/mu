package service

import (
	"os"
	"strings"
	"testing"
)

func TestBypassProxyForLoopbackSetsMissingProxyEnv(t *testing.T) {
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	bypassProxyForLoopback()

	for _, key := range []string{"NO_PROXY", "no_proxy"} {
		got := os.Getenv(key)
		if got == "" {
			t.Fatalf("%s was not set", key)
		}
		for _, want := range []string{"127.0.0.1", "localhost", "::1", "0.0.0.0"} {
			if !strings.Contains(got, want) {
				t.Fatalf("%s = %q, want it to include %q", key, got, want)
			}
		}
	}
}

func TestBypassProxyForLoopbackAppendsToExistingProxyEnv(t *testing.T) {
	t.Setenv("NO_PROXY", "example.com")
	t.Setenv("no_proxy", "internal.test")

	bypassProxyForLoopback()

	for key, prefix := range map[string]string{
		"NO_PROXY": "example.com,",
		"no_proxy": "internal.test,",
	} {
		got := os.Getenv(key)
		if !strings.HasPrefix(got, prefix) {
			t.Fatalf("%s = %q, want prefix %q", key, got, prefix)
		}
		if !strings.Contains(got, "127.0.0.1") || !strings.Contains(got, "localhost") {
			t.Fatalf("%s = %q, want appended loopback hosts", key, got)
		}
	}
}

func TestBypassProxyForLoopbackDoesNotDuplicateWhenLoopbackPresent(t *testing.T) {
	t.Setenv("NO_PROXY", "example.com,127.0.0.1")
	t.Setenv("no_proxy", "internal.test,127.0.0.1")

	bypassProxyForLoopback()

	for _, key := range []string{"NO_PROXY", "no_proxy"} {
		got := os.Getenv(key)
		if strings.Count(got, "127.0.0.1") != 1 {
			t.Fatalf("%s = %q, want a single 127.0.0.1 entry", key, got)
		}
	}
}
