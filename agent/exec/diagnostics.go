package exec

import (
	"encoding/json"
	"mu/agent"
	"mu/internal/ai"
	"mu/internal/app"
	"mu/service/tasks"
	"regexp"
	"strings"
)

var secretValue = regexp.MustCompile(`(?i)((?:password|secret|cookie|private[_-]?key|access[_-]?token|refresh[_-]?token)["']?\s*[:=]\s*["']?)[^\s"'&,}]+`)

func diagnosticText(s string) string {
	s = ai.ProviderErrorDetail(s)
	s = secretValue.ReplaceAllString(s, "${1}[redacted]")
	if len(s) > 16384 {
		s = s[:16384] + "\n[truncated]"
	}
	return strings.ToValidUTF8(s, "")
}
func recordedStep(s agent.Step, status string) tasks.Step {
	args, _ := json.Marshal(s.Args)
	return tasks.Step{ID: s.ID, Tool: s.Tool, Detail: diagnosticText(tasks.StepDetail(s.Args)), Args: diagnosticText(string(args)), Output: diagnosticText(s.Output), Error: diagnosticText(s.Error), OK: s.OK, Seconds: s.Took.Seconds(), Started: s.Started, Finished: s.Finished, Status: status}
}
func saveProgress(owner, id, run string, steps []tasks.Step, report, failure, version string) {
	if err := tasks.Progress(owner, id, run, steps, report, failure, version); err != nil {
		app.Log("work", "could not record progress for %s run %s: %v", id, run, err)
	}
}
