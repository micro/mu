package server

import (
	"encoding/json"
	"mu/internal/auth"
	"mu/service/tasks"
	"strings"
	"testing"
	"time"
)

func TestTasksContractOwnership(t *testing.T) {
	tasks.Load()
	const owner = "tasks_contract_owner"
	const other = "tasks_contract_other"
	for _, id := range []string{owner, other} {
		auth.SetAccountForTest(&auth.Account{ID: id, Admin: true})
		defer auth.RemoveAccountForTest(id)
	}
	token := func(id string, permissions ...string) string {
		_, value, err := auth.CreateToken(id, "tasks contract", permissions, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	own := token(owner, "read", "write", "service:tasks")
	foreign := token(other, "read", "write", "service:tasks")
	response := compositionCall(own, "tasks", "Tasks.Create", `{"title":"private task fixture","assignee":"me"}`)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var created struct {
		Item struct {
			ID string `json:"id"`
		} `json:"item"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil || created.Item.ID == "" {
		t.Fatal(err, response.Body.String())
	}
	body := `{"id":"` + created.Item.ID + `"}`
	defer compositionCall(own, "tasks", "Tasks.Delete", body)
	response = compositionCall(own, "tasks", "Tasks.List", `{"offset":"0","limit":"10"}`)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "private task fixture") {
		t.Fatal(response.Code, response.Body.String())
	}
	response = compositionCall(foreign, "tasks", "Tasks.Read", body)
	if response.Code != 200 || strings.Contains(response.Body.String(), "private task fixture") {
		t.Fatal(response.Code, response.Body.String())
	}
	for _, key := range []string{token(owner, "read", "service:tasks"), token(owner, "read", "write", "api:agent")} {
		response = compositionCall(key, "tasks", "Tasks.Delete", body)
		if response.Code != 403 {
			t.Fatal(response.Code, response.Body.String())
		}
	}
}
