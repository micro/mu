package server

import (
	"net/http/httptest"
	"testing"
)

func TestPublicFormsHaveNarrowCSRFException(t *testing.T) {
	for _, p := range []string{"/forms", "/forms/view", "/forms/received", "/forms/submit/extra"} {
		if csrfExempt(httptest.NewRequest("POST", p, nil)) {
			t.Fatal("management exempt", p)
		}
	}
	if !csrfExempt(httptest.NewRequest("POST", "/forms/submit?id=opaque", nil)) {
		t.Fatal("external form submission blocked")
	}
}
