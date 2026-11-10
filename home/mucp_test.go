package home

import (
	"mu/internal/auth"
	pb "mu/proto/assistant/v1"
	"mu/service/events"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMUCPHomeIncludesOnlyOwnedLocalEvents(t *testing.T) {
	owner := "home_contract_owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	when := time.Now().Add(time.Hour)
	if _, err := events.Create(owner, "My appointment", when, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := events.Create("home_contract_other", "Private appointment", when, ""); err != nil {
		t.Fatal(err)
	}
	out, err := MUCPOperations()[0].Handle(httptest.NewRecorder(), httptest.NewRequest("POST", "/", nil), owner, "request-123456789", &pb.HomeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range out.(*pb.HomeResponse).Events {
		if e.Title == "Private appointment" {
			t.Fatal("foreign event")
		}
		if e.Title == "My appointment" {
			found = true
		}
	}
	if !found {
		t.Fatal("local event missing")
	}
}
