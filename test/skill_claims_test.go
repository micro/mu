package test

// The skill under skills/mu is read by agents that have not tried anything yet.
// It is loaded before the first call and treated as authoritative, so a wrong
// claim in it is worse than no skill at all: the agent acts on it instead of
// discovering the truth.
//
// Prose has no compiler. These tests read the skill and check the parts that
// are mechanically checkable against the registry — the same job
// docs_claims_test.go does for docs/, and for the same reason.
//
// What they cannot check is whether the advice is good. That still needs a
// human.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"mu/internal/api"
	"mu/internal/service"
)

var skillDir = at("skills", "mu")

// skillText returns every markdown file in the skill, concatenated.
func skillText(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk(skillDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		c, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		b.WriteString("\n<!-- " + path + " -->\n")
		b.Write(c)
		return nil
	})
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	if b.Len() == 0 {
		t.Fatal("the skill has no markdown in it")
	}
	return b.String()
}

// The frontmatter is the only part loaded into every agent's system prompt at
// startup, and the description is what decides whether the rest is ever read.
// A skill whose frontmatter is malformed is silently not a skill.
func TestTheSkillHasFrontmatterAnAgentCanSelectOn(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	body := string(b)

	if !strings.HasPrefix(body, "---\n") {
		t.Fatal("SKILL.md does not open with YAML frontmatter")
	}
	end := strings.Index(body[4:], "\n---")
	if end < 0 {
		t.Fatal("the frontmatter is never closed")
	}
	front := body[4 : 4+end]

	for _, field := range []string{"name:", "description:"} {
		if !strings.Contains(front, field) {
			t.Errorf("the frontmatter has no %s, which the standard requires", field)
		}
	}

	// The same rule the tool surface is held to: a description that restates
	// the name tells a model choosing between skills nothing.
	for _, line := range strings.Split(front, "\n") {
		if strings.HasPrefix(line, "description:") {
			if d := strings.TrimSpace(strings.TrimPrefix(line, "description:")); len(d) < 80 {
				t.Errorf("the description is %d characters (%q) — too short to choose by", len(d), d)
			}
		}
	}
}

// Every tool the skill names must exist. An agent that tries one and gets
// "Tool not found" has been actively misled, and will reasonably distrust the
// rest of the skill.
func TestEveryToolTheSkillNamesIsReal(t *testing.T) {
	registerAll(t)
	// Registering a Spec does not by itself add its tools to the agent
	// surface; main() derives them in a second step, and so must this.
	api.DeriveTools()

	real := map[string]bool{}
	for _, tool := range api.Commands() {
		real[tool.Name] = true
		for _, a := range tool.Aliases {
			real[a] = true
		}
	}
	// Tools registered by hand rather than derived from a Spec are not in
	// this test binary's registry, so check them against the source that
	// registers them. Both are real registrations; neither can drift silently.
	mainGo := []byte(registrationSource(t))
	for _, m := range regexp.MustCompile(`Name:\s*"([a-z][a-z0-9_]*)"`).FindAllStringSubmatch(string(mainGo), -1) {
		real[m[1]] = true
	}

	// Tool names appear in backticks, either bare or followed by arguments.
	// `service_method` is the naming rule the skill states, not a tool.
	named := map[string]bool{}
	for _, m := range regexp.MustCompile("`([a-z][a-z0-9]*_[a-z0-9_]+)[ ({`]").FindAllStringSubmatch(skillText(t), -1) {
		if m[1] != "service_method" {
			named[m[1]] = true
		}
	}
	if len(named) < 20 {
		t.Fatalf("only found %d tool names in the skill — the pattern is probably wrong", len(named))
	}

	var missing []string
	for name := range named {
		if !real[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Errorf("the skill names %d tool(s) that do not exist: %s",
			len(missing), strings.Join(missing, ", "))
	}
}

// Prices are per-instance settings (CREDIT_COST_*), and a self-hosted Mu with
// payments unconfigured charges nothing at all. A number copied into the skill
// is wrong for somebody, and wrong in the direction that costs them money.
//
// The skill says so itself and points at wallet_check. This holds it to that.
func TestTheSkillQuotesNoPrices(t *testing.T) {
	text := skillText(t)

	// Strip the paragraphs that discuss pricing policy, which legitimately use
	// the word without quoting a figure.
	bad := regexp.MustCompile(`(?i)\b\d+\s*credits?\b`).FindAllString(text, -1)
	if len(bad) > 0 {
		t.Errorf("the skill quotes prices (%s) — they are instance settings and will be wrong somewhere; "+
			"teach wallet_check instead", strings.Join(bad, ", "))
	}
}

// The skill tells an agent which calls will be refused without a token. Getting
// this wrong wastes a call and, worse, teaches the agent to send a credential
// where none is needed.
func TestTheSkillIsRightAboutWhatNeedsAnAccount(t *testing.T) {
	registerAll(t)
	text := skillText(t)

	// Named as needing an account.
	for _, name := range []string{"mail", "contacts", "events", "tasks", "files", "wallet"} {
		if !service.AccountScoped(name) {
			t.Errorf("the skill says %s needs an account, but the service is not account-scoped", name)
		}
	}

	// Named as working anonymously. news and markets are the two the skill
	// leans on hardest for "answer locally before paying".
	for _, name := range []string{"news", "markets"} {
		if service.AccountScoped(name) {
			t.Errorf("the skill says %s works anonymously, but the service is account-scoped", name)
		}
	}

	// The refusal shape is the thing a client will get wrong, so the skill must
	// show it verbatim.
	if !strings.Contains(text, `{"error":"authentication required"}`) {
		t.Error("the skill does not show the refusal body, which is not a JSON-RPC error and is easy to misread")
	}
}
