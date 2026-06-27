package data

import (
	"errors"
	"reflect"
	"testing"
)

func resetDeleteRegistry(t *testing.T) {
	t.Helper()
	deleterMu.Lock()
	deleters = map[string]DeleteFunc{}
	deleterMu.Unlock()
}

func TestDeleteRegistryDeletesRegisteredContent(t *testing.T) {
	resetDeleteRegistry(t)

	var deletedID string
	RegisterDeleter("post", func(id string) error {
		deletedID = id
		return nil
	})

	if err := Delete("post", "post-123"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if deletedID != "post-123" {
		t.Fatalf("deleter called with id %q, want post-123", deletedID)
	}
}

func TestDeleteRegistryReportsMissingAndPropagatesErrors(t *testing.T) {
	resetDeleteRegistry(t)

	if err := Delete("unknown", "item-1"); err == nil {
		t.Fatal("Delete returned nil for an unregistered content type")
	}

	wantErr := errors.New("delete failed")
	RegisterDeleter("post", func(id string) error { return wantErr })
	if err := Delete("post", "post-123"); !errors.Is(err, wantErr) {
		t.Fatalf("Delete error = %v, want %v", err, wantErr)
	}
}

func TestDeleteTypesReturnsSortedContentTypes(t *testing.T) {
	resetDeleteRegistry(t)

	RegisterDeleter("video", func(string) error { return nil })
	RegisterDeleter("post", func(string) error { return nil })
	RegisterDeleter("article", func(string) error { return nil })

	got := DeleteTypes()
	want := []string{"article", "post", "video"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DeleteTypes() = %v, want %v", got, want)
	}
}
