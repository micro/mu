package agent

import (
	"mu/internal/model"
	"reflect"
	"strings"
	"testing"
)

func TestCompositionControlsAgent(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "http://localhost:12345/v1")
	t.Setenv("AGENT_MODEL", "ignored-instance-default")
	m := model.Default()
	m.LLM = model.LLM{Provider: "openai", ID: "chosen-model"}
	m.Instructions = "Composition instructions"
	m.Services = []string{"news", "mail"}
	opts := QueryOpts{Definition: &m, Tools: []string{"news", "weather"}}
	p, _, id, base, ok := nativeModelForAccount("", opts)
	if base != "http://localhost:12345" {
		t.Fatalf("base URL was not normalized: %q", base)
	}
	if !ok || p != "openai" || id != "chosen-model" {
		t.Fatalf("selection %q %q %v", p, id, ok)
	}
	if !strings.Contains(nativeSystem(opts), m.Instructions) {
		t.Fatal("composition instructions missing")
	}
	if got := selectedServices(opts, []string{"news", "weather"}); !reflect.DeepEqual(got, []string{"news"}) {
		t.Fatalf("services %v", got)
	}
	m.Services = []string{"mail"}
	if got := selectedServices(opts, []string{"news", "weather"}); len(got) != 0 {
		t.Fatalf("scope widened: %v", got)
	}
	m.Services = []string{}
	if got := selectedServices(opts, []string{"news"}); len(got) != 0 {
		t.Fatal("empty selection enabled tools")
	}
	m.Services = nil
	opts.NoTools = true
	if len(selectedServices(opts, []string{"news"})) != 0 {
		t.Fatal("NoTools ignored")
	}
	m.LLM.Provider = "unknown"
	if _, _, _, _, ok := nativeModelForAccount("", opts); ok {
		t.Fatal("invalid provider fell back")
	}
}
