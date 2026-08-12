package contacts

// The service half of the address book tests: identity comes from the call
// context, never a request field. The store's own tests live with the store, in
// internal/contacts.

import (
	"context"
	"os"
	"testing"

	"mu/internal/service"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-contacts-service-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// clear empties an owner's address book between tests.
func clear(owner string) {
	for _, c := range List(owner) {
		_ = Remove(owner, c.ID)
	}
}

func TestServiceBindsOwnerFromContext(t *testing.T) {
	clear("carol")
	defer clear("carol")

	var add AddResponse
	err := Server{}.Add(service.WithAccount(context.Background(), "carol"),
		&AddRequest{Name: "Dan", Email: "dan@example.com"}, &add)
	if err != nil {
		t.Fatal(err)
	}
	if add.Contact.Owner != "carol" {
		t.Errorf("owner is %q, want carol", add.Contact.Owner)
	}

	if err := (Server{}).List(context.Background(), &ListRequest{}, &ListResponse{}); err == nil {
		t.Error("a caller with no account listed contacts")
	}
}
