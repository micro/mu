package work

import (
	"encoding/json"
	"fmt"
	"strings"
)

const outcomeInstruction = `Execute the task using the available tools, checking their actual results. Keep working until the requested outcome is verified or you encounter a concrete blocker. A successful read, static check or shell transport is not evidence that a requested change was made. Do not invent verification or claim success for a non-zero shell exit code.
Return your final report as one JSON object (no surrounding prose):
{"status":"done","summary":"What was delivered, including its URL when relevant","evidence":["Specific checks performed and their observed results"]}
Use status "blocked" with a summary explaining the missing capability, failed verification or work remaining when you cannot finish. Do not request a new task for the same work. Evidence is required for done, including read-only tasks; it must describe observed results, not future intentions. This reporting contract is not permission to perform actions beyond the user's request.`

type blockedOutcome struct{ summary string }

func (e *blockedOutcome) Error() string { return e.summary }

// readOutcome validates the report's structure, not the truth of model claims.
// Keeping that distinction lets clients inspect the evidence independently.
func readOutcome(reply string) (string, error) {
	var report struct {
		Status   string   `json:"status"`
		Summary  string   `json:"summary"`
		Evidence []string `json:"evidence"`
	}
	reply = strings.TrimSpace(reply)
	if strings.HasPrefix(reply, "```json\n") && strings.HasSuffix(reply, "```") {
		reply = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(reply, "```json\n"), "```"))
	}
	if err := json.Unmarshal([]byte(reply), &report); err != nil || strings.TrimSpace(report.Summary) == "" {
		return "", fmt.Errorf("the agent did not return a valid task outcome; completion is unverified")
	}
	summary := strings.TrimSpace(report.Summary)
	if report.Status == "blocked" {
		return summary, &blockedOutcome{summary: summary}
	}
	if report.Status != "done" {
		return "", fmt.Errorf("the agent did not report a supported task status; completion is unverified")
	}
	var evidence []string
	for _, item := range report.Evidence {
		if item = strings.TrimSpace(item); item != "" {
			evidence = append(evidence, item)
		}
	}
	if len(evidence) == 0 {
		return summary, &blockedOutcome{summary: summary + "\n\nCompletion is unverified: the agent supplied no verification evidence."}
	}
	return summary + "\n\nReported checks:\n- " + strings.Join(evidence, "\n- "), nil
}
