package api

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResourceIDsAcrossFormats(t *testing.T) {
	for _, tc := range []struct {
		path, query, media, body, want string
		ok                             bool
	}{
		{"123", "", "", "", "123", true},
		{"", "id=123", "", "", "123", true},
		{"", "", "application/json", `{"id":"123"}`, "123", true},
		{"", "", "application/x-www-form-urlencoded", "id=123", "123", true},
		{"123", "id=123", "application/json", `{"id":"123","action":"retry"}`, "123", true},
		{"123", "id=456", "", "", "", false},
		{"", "id=123&id=456", "", "", "", false},
		{"123", "", "application/json", `{"id":"456"}`, "", false},
		{"123", "", "application/x-www-form-urlencoded", "id=456", "", false},
		{"", "", "application/json", `{"id":123}`, "", false},
		{"", "", "application/json", `{"id":"123","id":"456"}`, "", false},
		{"", "", "application/json", `{"id":"123"} {}`, "", false},
	} {
		t.Run(tc.path+tc.query+tc.body, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/work?"+tc.query, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.media)
			w := httptest.NewRecorder()
			got, ok := ResourceID(w, r, tc.path)
			if ok != tc.ok {
				t.Fatalf("ok %v: %s", ok, w.Body.String())
			}
			if !ok {
				if w.Code != 400 {
					t.Fatal(w.Code)
				}
				return
			}
			if got.URL.Query().Get("id") != tc.want || got.Method != "POST" {
				t.Fatal("resolution changed operation")
			}
			if tc.media == "application/json" {
				data, _ := io.ReadAll(got.Body)
				if string(data) != tc.body {
					t.Fatal("body lost")
				}
			}
		})
	}
}
