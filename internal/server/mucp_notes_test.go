package server

import (
	"mu/internal/auth"
	"mu/service/notes"
	"strings"
	"testing"
	"time"
)

func TestNotesContractOwnership(t *testing.T) {
	notes.LoadService()
	tokens := map[string]string{}
	for _, id := range []string{"notes_contract_owner", "notes_contract_other"} {
		auth.SetAccountForTest(&auth.Account{ID: id, Admin: true})
		defer auth.RemoveAccountForTest(id)
		_, token, err := auth.CreateToken(id, "notes", []string{"read", "write", "service:notes"}, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		tokens[id] = token
	}
	owner := tokens["notes_contract_owner"]
	response := compositionCall(owner, "notes", "Notes.Add", `{"title":"contract fixture","text":"private fixture"}`)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	defer compositionCall(owner, "notes", "Notes.Delete", `{"title":"contract fixture"}`)
	response = compositionCall(owner, "notes", "Notes.List", `{}`)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "private fixture") {
		t.Fatal(response.Code, response.Body.String())
	}
	response = compositionCall(tokens["notes_contract_other"], "notes", "Notes.List", `{}`)
	if response.Code != 200 || strings.Contains(response.Body.String(), "private fixture") {
		t.Fatal(response.Code, response.Body.String())
	}
	_, readonly, _ := auth.CreateToken("notes_contract_owner", "read only", []string{"read", "service:notes"}, time.Time{})
	response = compositionCall(readonly, "notes", "Notes.Delete", `{"title":"contract fixture"}`)
	if response.Code != 403 {
		t.Fatal(response.Code, response.Body.String())
	}
	_, assistant, _ := auth.CreateToken("notes_contract_owner", "assistant", []string{"read", "write", "api:agent"}, time.Time{})
	response = compositionCall(assistant, "notes", "Notes.List", `{}`)
	if response.Code != 403 {
		t.Fatal(response.Code, response.Body.String())
	}
	response = compositionCall(owner, "notes", "Notes.Add", `{"title":"contract fixture","text":"private fixture","account":"other"}`)
	if response.Code != 400 {
		t.Fatal(response.Code, response.Body.String())
	}
}
