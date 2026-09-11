package agent

import (
	"encoding/json"

	gmai "go-micro.dev/v6/model"
)

// A successful tool transport can still contain a failed operation. Shell
// retains its output on failure, so inspect the exit code without discarding it.
func toolStepSucceeded(name string, result gmai.ToolResult) bool {
	if result.Refused != "" || toolResultError(result) != "" {
		return false
	}
	if NativeToolName(name) != "shell_run" {
		return true
	}
	payload := []byte(result.Content)
	if result.Value != nil {
		if encoded, err := json.Marshal(result.Value); err == nil {
			payload = encoded
		}
	}
	var shell struct {
		Code *int `json:"code"`
	}
	return json.Unmarshal(payload, &shell) == nil && shell.Code != nil && *shell.Code == 0
}
