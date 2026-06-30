package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"mu/internal/settings"
)

// runSetup is the `mu setup` wizard: a headless-friendly companion to the web
// /setup page. It writes the AI provider into the shared settings store so the
// next `mu --serve` comes up configured. Admin creation happens on first web
// signup (or via the ADMIN env var), which it explains at the end.
func runSetup(_ []string) int {
	settings.Load() // don't clobber existing settings

	in := bufio.NewReader(os.Stdin)
	fmt.Println("Mu setup — configure an AI provider.")
	fmt.Println()
	fmt.Println("  1) Anthropic Claude")
	fmt.Println("  2) Atlas Cloud / DeepSeek")
	fmt.Println("  3) Ollama / OpenAI-compatible (local)")
	choice := prompt(in, "Provider [1-3]: ")

	switch choice {
	case "1":
		key := prompt(in, "Anthropic API key: ")
		if key == "" {
			return setupErr("no key entered")
		}
		settings.Set("ANTHROPIC_API_KEY", key)
	case "2":
		key := prompt(in, "Atlas Cloud API key: ")
		if key == "" {
			return setupErr("no key entered")
		}
		settings.Set("ATLAS_API_KEY", key)
	case "3":
		url := prompt(in, "Base URL [http://localhost:11434/v1]: ")
		if url == "" {
			url = "http://localhost:11434/v1"
		}
		settings.Set("OPENAI_BASE_URL", url)
		settings.Set("OPENAI_API_KEY", "ollama")
	default:
		return setupErr("pick 1, 2 or 3")
	}

	fmt.Println()
	fmt.Println("✓ AI provider saved.")
	fmt.Println()
	fmt.Println("Next:")
	fmt.Println("  mu --serve            # start the server")
	fmt.Println("  open http://localhost:8080")
	fmt.Println()
	fmt.Println("The first account you create becomes admin")
	fmt.Println("(or set ADMIN=you@example.com before starting the server).")
	return 0
}

func prompt(in *bufio.Reader, label string) string {
	fmt.Print(label)
	line, _ := in.ReadString('\n')
	return strings.TrimSpace(line)
}

func setupErr(msg string) int {
	fmt.Fprintln(os.Stderr, "setup:", msg)
	return 2
}
