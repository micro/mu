package flow

import (
	"context"
	"fmt"

	"mu/tools"
)

func registerTools() {
	// flow.create - Create a new flow from source
	tools.Register(tools.Tool{
		Name:        "flow.create",
		Description: "Create a new automation flow",
		Category:    "flow",
		Input: map[string]tools.Param{
			"name":   {Type: "string", Description: "Name for the flow", Required: true},
			"source": {Type: "string", Description: "Flow source code", Required: true},
		},
		Output: map[string]tools.Param{
			"id":   {Type: "string", Description: "Flow ID"},
			"name": {Type: "string", Description: "Flow name"},
			"url":  {Type: "string", Description: "URL to view the flow"},
		},
		Handler: handleFlowCreate,
	})

	// flow.list - List user's flows
	tools.Register(tools.Tool{
		Name:        "flow.list",
		Description: "List user's automation flows",
		Category:    "flow",
		Input:       map[string]tools.Param{},
		Output: map[string]tools.Param{
			"flows": {Type: "array", Description: "List of flows"},
		},
		Handler: handleFlowList,
	})

	// flow.run - Execute a flow
	tools.Register(tools.Tool{
		Name:        "flow.run",
		Description: "Run a flow by ID",
		Category:    "flow",
		Input: map[string]tools.Param{
			"id": {Type: "string", Description: "Flow ID to run", Required: true},
		},
		Output: map[string]tools.Param{
			"success": {Type: "bool", Description: "Whether the flow ran successfully"},
			"result":  {Type: "object", Description: "Execution result"},
		},
		Handler: handleFlowRun,
	})

	// flow.delete - Delete a flow
	tools.Register(tools.Tool{
		Name:        "flow.delete",
		Description: "Delete a flow",
		Category:    "flow",
		Input: map[string]tools.Param{
			"id": {Type: "string", Description: "Flow ID to delete", Required: true},
		},
		Output: map[string]tools.Param{
			"success": {Type: "bool", Description: "Whether the flow was deleted"},
		},
		Handler: handleFlowDelete,
	})
}

func handleFlowCreate(ctx context.Context, params map[string]any) (any, error) {
	userID := tools.UserFromContext(ctx)
	if userID == "" {
		return nil, fmt.Errorf("not authenticated")
	}

	name, _ := params["name"].(string)
	source, _ := params["source"].(string)

	if name == "" || source == "" {
		return nil, fmt.Errorf("name and source are required")
	}

	f, err := Create(userID, name, source)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"id":   f.ID,
		"name": f.Name,
		"url":  "/flows/" + f.ID,
	}, nil
}

func handleFlowList(ctx context.Context, params map[string]any) (any, error) {
	userID := tools.UserFromContext(ctx)
	if userID == "" {
		return nil, fmt.Errorf("not authenticated")
	}

	flows := ListByUser(userID)
	result := make([]map[string]any, len(flows))
	for i, f := range flows {
		result[i] = map[string]any{
			"id":       f.ID,
			"name":     f.Name,
			"enabled":  f.Enabled,
			"schedule": f.Schedule,
			"run_count": f.RunCount,
		}
	}

	return map[string]any{"flows": result}, nil
}

func handleFlowRun(ctx context.Context, params map[string]any) (any, error) {
	userID := tools.UserFromContext(ctx)
	if userID == "" {
		return nil, fmt.Errorf("not authenticated")
	}

	id, _ := params["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("flow id required")
	}

	f := Get(id)
	if f == nil {
		return nil, fmt.Errorf("flow not found")
	}

	if f.UserID != userID {
		return nil, fmt.Errorf("not authorized")
	}

	result := Execute(f, userID)
	return map[string]any{
		"success": result.Success,
		"result":  result,
	}, nil
}

func handleFlowDelete(ctx context.Context, params map[string]any) (any, error) {
	userID := tools.UserFromContext(ctx)
	if userID == "" {
		return nil, fmt.Errorf("not authenticated")
	}

	id, _ := params["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("flow id required")
	}

	f := Get(id)
	if f == nil {
		return nil, fmt.Errorf("flow not found")
	}

	if f.UserID != userID {
		return nil, fmt.Errorf("not authorized")
	}

	err := f.Delete()
	return map[string]any{"success": err == nil}, err
}
