package agent

// How a run went, as opposed to what it cost.
//
// # Why this exists
//
// recordRunCost already loads every run's full timeline the moment the run
// ends — go-micro records a line per model call and per tool call, with the
// status, the error, the latency, the retry count and the tokens — and it reads
// the tokens. Everything else was loaded into memory and dropped.
//
// So the question "is Micro reliable" had no answer anywhere in the product.
// Not a hard answer, not a rough one: the instance could say what it had spent
// and not whether the answers were arriving. A run that hit the step cap, a
// tool that failed on every call, a provider retrying three times before each
// reply — all of it happened, was recorded by the framework, was read by this
// package, and was thrown away.
//
// # What it is for
//
// Two decisions, both of which were being argued from intuition.
//
// The first is the tool catalogue. A hundred and nineteen tools are offered to
// every signed-in question, and the case for narrowing that per question rests
// entirely on whether the model takes wrong turns among them. That is a
// measurement — steps per run, and which tools got called — and nobody had it.
//
// The second is where to spend effort. "Micro needs to be bulletproof" is a
// direction, and the top three errors of the last hundred runs is the list that
// turns it into work.
//
// # Bounded, in memory, and not a record of anybody
//
// A hundred runs, in a ring, lost on restart. This is an operator's view of how
// the machine is behaving, not a history: the history is the conversation, in
// internal/thread, which belongs to whoever had it. Nothing here carries an
// account or a question — a tool name and a status say how the loop went
// without saying whose loop it was, and that is the whole of what an operator
// needs to fix it.

import (
	"sort"
	"strings"
	"sync"
	"time"

	gmagent "go-micro.dev/v6/agent"
)

// An Outcome is one run, as the operator sees it.
type Outcome struct {
	At      time.Time
	Agent   string   // which agent ran — "micro", "code"
	Caller  string   // "agent" or "agent_guest", the same split the spend log uses
	Steps   int      // model calls, which is how many times round the loop
	Tools   []string // the tools it called, in order, with repeats
	Failed  int      // tool calls that came back an error
	Retries int      // model calls that were not the first attempt
	Status  string   // the run's own status: done, timeout, refused, error…
	Error   string   // the first error, when there was one
	Latency int64    // milliseconds of model time, summed
}

// keptRuns is how many runs are kept.
//
// A hundred. Enough to see a pattern and small enough that this is a fixed cost
// on a box doing nothing else — the same reasoning as internal/world's fifty,
// and for the same reason it is a ring rather than a log.
const keptRuns = 100

var (
	outcomeMu sync.RWMutex
	outcomes  []Outcome // oldest first
)

// recordOutcome keeps how one run went.
//
// Called from the same place and with the same timeline as recordRunCost. Two
// functions rather than one because they answer to different readers — a bill
// and a diagnosis — and a function that did both would be read as neither.
func recordOutcome(events []gmagent.RunEvent, agentName, caller string) {
	if len(events) == 0 {
		return
	}
	o := Outcome{At: time.Now().UTC(), Agent: agentName, Caller: caller}
	for _, e := range events {
		switch e.Kind {
		case "model", "stream":
			o.Steps++
			if e.Attempt > 1 {
				// A retried model call. Worth counting separately from a
				// failure: a run that answered after three attempts looks
				// perfect from outside and is a provider in trouble.
				o.Retries++
			}
		case "tool":
			o.Tools = append(o.Tools, e.Name)
			if strings.TrimSpace(e.Error) != "" {
				o.Failed++
			}
		}
		o.Latency += e.LatencyMS
		if e.Status != "" && e.Status != "done" && e.Status != "running" {
			o.Status = e.Status
		}
		if o.Error == "" {
			// The first one, not the last. A run that fails once and then
			// cascades reports its cause here rather than its final symptom.
			if msg := strings.TrimSpace(e.Error); msg != "" {
				o.Error = msg
			}
		}
	}
	if o.Status == "" {
		o.Status = "done"
	}

	outcomeMu.Lock()
	defer outcomeMu.Unlock()
	outcomes = append(outcomes, o)
	if len(outcomes) > keptRuns {
		outcomes = outcomes[len(outcomes)-keptRuns:]
	}
}

// Outcomes is how the last runs went, newest first.
func Outcomes() []Outcome {
	outcomeMu.RLock()
	defer outcomeMu.RUnlock()
	out := make([]Outcome, 0, len(outcomes))
	for i := len(outcomes) - 1; i >= 0; i-- {
		out = append(out, outcomes[i])
	}
	return out
}

// Health is the summary an operator reads first.
type Health struct {
	Runs       int
	Failed     int     // runs that did not end done
	ToolErrors int     // tool calls that came back an error
	Retried    int     // runs with at least one retried model call
	MedianStep int     // steps in the middle run
	MaxStep    int     // the longest run's steps
	TopTools   []Count // what actually gets called
	TopErrors  []Count // and what actually goes wrong
}

// A Count is one name and how often it came up.
type Count struct {
	Name string
	N    int
}

// Summary folds the runs into the two lists that turn "make it reliable" into
// work: what gets called, and what goes wrong.
//
// The tool counts are the evidence for or against narrowing the catalogue. A
// hundred and nineteen tools are offered; if eleven of them account for
// everything, the offer is the thing to change. If the tail is genuinely used,
// it is not.
func Summary() Health {
	runs := Outcomes()
	h := Health{Runs: len(runs)}
	if len(runs) == 0 {
		return h
	}
	byTool := map[string]int{}
	errs := map[string]int{}
	steps := make([]int, 0, len(runs))
	for _, r := range runs {
		if r.Status != "done" {
			h.Failed++
		}
		h.ToolErrors += r.Failed
		if r.Retries > 0 {
			h.Retried++
		}
		steps = append(steps, r.Steps)
		if r.Steps > h.MaxStep {
			h.MaxStep = r.Steps
		}
		for _, t := range r.Tools {
			byTool[t]++
		}
		if r.Error != "" {
			errs[r.Error]++
		}
	}
	sort.Ints(steps)
	h.MedianStep = steps[len(steps)/2]
	h.TopTools = top(byTool, 12)
	h.TopErrors = top(errs, 5)
	return h
}

// top is the n most common, most first, ties broken by name so two reads of the
// same data agree.
func top(m map[string]int, n int) []Count {
	out := make([]Count, 0, len(m))
	for k, v := range m {
		out = append(out, Count{Name: k, N: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// ForgetOutcomes empties the record. For tests, and for an operator who has
// just changed something and wants the next hundred runs to be the ones they
// are reading.
func ForgetOutcomes() {
	outcomeMu.Lock()
	outcomes = nil
	outcomeMu.Unlock()
}
