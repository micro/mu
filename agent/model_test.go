package agent

import (
	"strings"
	"testing"
)

// An agent's model reaches the run.
//
// This is the wiring that was missing, and it was missing in the quietest
// possible way: QueryOpts.Model has always been carried down to nativeLLMFor,
// micro.Agent has always had the field, and PlatformOpts has always passed it —
// but AskAs, the path every one of your own agents takes, built its QueryOpts
// with System and Tools and dropped the model on the floor. Set it and the run
// used the instance default anyway, with nothing on screen to say which had
// happened.
func TestYourOwnAgentsModelReachesTheRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ATLAS_API_KEY", "atlas-test")

	const owner = "picker"
	a, _, err := CreateAgent(owner, "Research", Hosted, "Find things out.", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetModel(owner, a.ID, "deepseek-ai/deepseek-v4-flash"); err != nil {
		t.Fatal(err)
	}

	opts, err := AskAs(owner, "Research")
	if err != nil {
		t.Fatal(err)
	}
	if opts.Model != "deepseek-ai/deepseek-v4-flash" {
		t.Errorf("the run was given model %q; the agent is set to the flash one", opts.Model)
	}
	// And the rest still arrives, so this did not trade one dropped field for
	// another.
	if strings.TrimSpace(opts.System) == "" {
		t.Error("the standing instruction was dropped")
	}
}

// A model this instance cannot run is refused where somebody can fix it.
//
// Not at the model call, which happens minutes later on a run somebody is
// waiting for, by which time the person who chose it has gone.
func TestAModelWeCannotRunIsRefused(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ATLAS_API_KEY", "atlas-test")

	const owner = "picker2"
	a, _, err := CreateAgent(owner, "Research", Hosted, "Find things out.", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}

	// No Anthropic key here, so a Claude id is a run that cannot happen.
	if err := SetModel(owner, a.ID, "claude-sonnet-5"); err == nil {
		t.Error("a model with no provider behind it was accepted")
	}
	// And nothing was stored, so a refused choice does not half-apply.
	if got := For(owner, a.ID); got.Model != "" {
		t.Errorf("the refused model was stored anyway: %q", got.Model)
	}
}

// Clearing it goes back to the instance default, which is what an agent has
// before anybody chooses and what it must return to when a provider is removed.
func TestClearingTheModelIsAllowed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ATLAS_API_KEY", "atlas-test")

	const owner = "picker3"
	a, _, err := CreateAgent(owner, "Research", Hosted, "Find things out.", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetModel(owner, a.ID, "deepseek-ai/deepseek-v4-flash"); err != nil {
		t.Fatal(err)
	}
	if err := SetModel(owner, a.ID, ""); err != nil {
		t.Fatalf("clearing the model was refused: %v", err)
	}
	if got := For(owner, a.ID); got.Model != "" {
		t.Errorf("the model is still %q after clearing", got.Model)
	}
}

// Editing an agent leaves its model alone. UpdateAgent rewrites the prompt, the
// description and the scope, and a field it does not know about must survive
// that — otherwise renaming an agent silently resets what it thinks with.
func TestEditingDoesNotClearTheModel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ATLAS_API_KEY", "atlas-test")

	const owner = "picker4"
	a, _, err := CreateAgent(owner, "Research", Hosted, "Find things out.", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetModel(owner, a.ID, "deepseek-ai/deepseek-v4-flash"); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateAgent(owner, a.ID, "Research", "Find things out, carefully.", "", nil); err != nil {
		t.Fatal(err)
	}
	if got := For(owner, a.ID); got.Model != "deepseek-ai/deepseek-v4-flash" {
		t.Errorf("editing reset the model to %q", got.Model)
	}
}
