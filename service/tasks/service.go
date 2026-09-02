package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/service"
)

// Server is the go-micro service handler for tasks.
type Server struct{}

// CreateRequest adds a task.
type CreateRequest struct {
	Title    string `json:"title" required:"true" description:"What is to be done"`
	Detail   string `json:"detail" description:"Anything the doer needs to know: context, links, constraints"`
	Assignee string `json:"assignee" description:"me (default) or agent — assign to the agent and it can pick the task up"`
	Due      string `json:"due" description:"Optional deadline, RFC3339 or 2006-01-02 15:04"`
}

// TaskResponse carries one task back.
type TaskResponse struct {
	Text string `json:"text" description:"The task"`
}

// ListRequest filters the caller's tasks.
type ListRequest struct {
	Status string `json:"status" description:"Optional filter: todo, doing or done"`
}

// ListResponse is a model-ready list.
type ListResponse struct {
	Text string `json:"text" description:"The caller's tasks, open ones first"`
}

// NextRequest takes nothing: the next task is a property of the list.
type NextRequest struct{}

// UpdateRequest changes a task. Empty fields are left alone.
type UpdateRequest struct {
	ID     string `json:"id" description:"The task's id" required:"true"`
	Title  string `json:"title" description:"New title"`
	Detail string `json:"detail" description:"New detail"`
	Status string `json:"status" description:"todo, doing or done"`
	Result string `json:"result" description:"What came of it — the answer, the outcome, what was found"`
}

// DeleteRequest removes a task.
type DeleteRequest struct {
	ID string `json:"id" description:"The task's id" required:"true"`
}

// DeleteResponse confirms the removal.
type DeleteResponse struct {
	Text string `json:"text" description:"Confirmation"`
}

// Create adds a task to the caller's list.
// @example {"title": "Summarise the week's AI news", "assignee": "agent"}
func (Server) Create(ctx context.Context, req *CreateRequest, rsp *TaskResponse) error {
	owner := service.AccountFrom(ctx)
	due, err := ParseDue(req.Due)
	if err != nil {
		return err
	}
	t, err := Create(owner, req.Title, req.Detail, req.Assignee, due)
	if err != nil {
		return err
	}

	// Assigned to the agent means the agent starts on it.
	//
	// This endpoint has said "assign it to the agent and it can pick the task
	// up itself" since it was written, and nothing picked anything up: Run is
	// what announces the work, and its only caller was a button on /inbox. So
	// "file this for the agent" put a row in a list and waited for somebody to
	// press a thing they had no reason to know about.
	//
	// Not when the agent filed it. A model that creates a task for itself
	// mid-run is describing what it is already doing; starting a second run
	// from inside the first is how one question becomes a loop, and Run's
	// status guard does not catch it because each turn creates a *new* task.
	// The distinction is who called, which is why it is on the context and not
	// in the request — see service.InAgentRun.
	//
	// In the background, because this call should return the task rather than
	// wait for the work: Run marks it doing and announces, and /tasks is the
	// progress indicator.
	if strings.EqualFold(strings.TrimSpace(req.Assignee), Agent) && !service.InAgentRun(ctx) {
		go func() {
			if err := Run(owner, t.ID); err != nil {
				app.Log("tasks", "starting %s for %s: %v", t.ID, owner, err)
			}
		}()
	}

	rsp.Text = Render([]*Task{t})
	return nil
}

// List returns the caller's tasks.
// @example {"status": "todo"}
func (Server) List(ctx context.Context, req *ListRequest, rsp *ListResponse) error {
	rsp.Text = Render(List(service.AccountFrom(ctx), req.Status))
	return nil
}

// Next returns the next task assigned to the agent.
// @example {}
func (Server) Next(ctx context.Context, _ *NextRequest, rsp *TaskResponse) error {
	t := Next(service.AccountFrom(ctx))
	if t == nil {
		rsp.Text = "Nothing assigned to the agent."
		return nil
	}
	rsp.Text = Render([]*Task{t})
	return nil
}

// Update changes a task — its state, or what came of it.
// @example {"id": "abc123", "status": "done", "result": "Mailed the summary"}
func (Server) Update(ctx context.Context, req *UpdateRequest, rsp *TaskResponse) error {
	t, err := Update(service.AccountFrom(ctx), req.ID, req.Title, req.Detail, req.Status, "", req.Result)
	if err != nil {
		return err
	}
	rsp.Text = Render([]*Task{t})
	return nil
}

// Delete removes a task.
// @example {"id": "abc123"}
func (Server) Delete(ctx context.Context, req *DeleteRequest, rsp *DeleteResponse) error {
	if err := Remove(service.AccountFrom(ctx), req.ID); err != nil {
		return err
	}
	rsp.Text = "Task removed."
	return nil
}

// ParseDue accepts the shapes a caller plausibly sends. Exported because the
// MCP tool takes a due date as a string too, and two parsers would disagree.
func ParseDue(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("could not read %q as a date", s)
}

func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("tasks", "service register failed: %v", err)
	}

	// The index is a separate file from the store, so an instance with one and
	// not the other answers every search with nothing — including every
	// instance upgrading to the first build that indexes tasks. In the
	// background: a boot-time cost proportional to the work recorded, and
	// nothing needs it before the first search.
	go Reindex()
}

var Spec = service.Spec{
	Name:        "tasks",
	Handler:     new(Server),
	Description: "What is to be done: a list you keep, and work you can hand to the agent",
	Page:        "/tasks",
	Icon:        "tasks.svg",
	Scoped:      true,
	Endpoints: map[string]service.Endpoint{
		"Create": {Writes: true, Doc: "Add a task. Assign it to the agent and it can pick the task up itself"},
		"List":   {Doc: "List the caller's tasks, open ones first; optionally filtered by state"},
		"Next":   {Doc: "The next task assigned to the agent — what to work on now"},
		"Update": {Writes: true, Doc: "Change a task: its state, or the result of doing it"},
		"Delete": {Doc: "Remove a task", Destructive: true},
	},
}
