// `mu login` and `mu logout` — store or clear the Personal Access
// Token used to authenticate against the Mu MCP endpoint.
package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// runLogin handles `mu login [--url URL]`. It offers to open the
// browser to the token page, then reads the pasted token from stdin.
func runLogin(args []string, cfg *ResolvedConfig) int {
	file, _ := LoadConfig()
	if file == nil {
		file = &Config{}
	}

	// Figure out the target URL. Respect flag/env override, otherwise
	// reuse whatever is stored (falling back to the default).
	url := cfg.URL
	if url == "" {
		url = file.URL
	}
	if url == "" {
		url = DefaultURL
	}

	fmt.Fprintf(os.Stdout, "Logging in to %s\n", url)
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "1. Sign in and create a Personal Access Token at:")
	fmt.Fprintf(os.Stdout, "   %s/token\n", url)
	fmt.Fprintln(os.Stdout)

	// Try to open the token page in a browser, but don't fail the
	// command when there is no browser available (SSH sessions,
	// containers, etc.).
	if err := openBrowser(url + "/token"); err != nil {
		fmt.Fprintf(os.Stdout, "   (couldn't open browser automatically — open the URL manually)\n")
	}

	fmt.Fprintln(os.Stdout, "2. Paste the token below and press Enter.")
	fmt.Fprint(os.Stdout, "Token: ")

	token, err := readLine(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to read token:", err)
		return 1
	}
	token = strings.TrimSpace(token)
	if token == "" {
		fmt.Fprintln(os.Stderr, "no token entered")
		return 1
	}

	file.URL = url
	file.Token = token
	if err := SaveConfig(file); err != nil {
		fmt.Fprintln(os.Stderr, "save config:", err)
		return 1
	}

	// Verify the token actually works.
	rc := &ResolvedConfig{URL: url, Token: token}
	client := NewClient(rc)
	if _, err := client.CallTool("me", nil); err != nil {
		fmt.Fprintln(os.Stderr, "warning: token saved but verification failed:", err)
		return 0
	}
	fmt.Fprintln(os.Stdout, "✓ Logged in")
	return 0
}

// runLogout clears the stored token.
func runLogout(args []string, cfg *ResolvedConfig) int {
	file, err := LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "load config:", err)
		return 1
	}
	if file == nil {
		file = &Config{}
	}
	file.Token = ""
	if err := SaveConfig(file); err != nil {
		fmt.Fprintln(os.Stderr, "save config:", err)
		return 1
	}
	fmt.Fprintln(os.Stdout, "✓ Logged out")
	return 0
}

// runConfig handles `mu config get|set|path`.
func runConfig(args []string, cfg *ResolvedConfig) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: mu config <get|set|path> [key] [value]")
		return 2
	}
	switch args[0] {
	case "path":
		p, err := configPath()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println(p)
		return 0
	case "get":
		file, err := LoadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if len(args) < 2 {
			fmt.Printf("url=%s\n", file.URL)
			if file.Token != "" {
				fmt.Println("token=***")
			} else {
				fmt.Println("token=")
			}
			return 0
		}
		switch args[1] {
		case "url":
			fmt.Println(file.URL)
		case "token":
			fmt.Println(file.Token)
		default:
			fmt.Fprintln(os.Stderr, "unknown key:", args[1])
			return 2
		}
		return 0
	case "set":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: mu config set <url|token> <value>")
			return 2
		}
		file, err := LoadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		switch args[1] {
		case "url":
			file.URL = args[2]
		case "token":
			file.Token = args[2]
		default:
			fmt.Fprintln(os.Stderr, "unknown key:", args[1])
			return 2
		}
		if err := SaveConfig(file); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("✓ Saved")
		return 0
	}
	fmt.Fprintln(os.Stderr, "usage: mu config <get|set|path> [key] [value]")
	return 2
}

// readLine reads a single line from a reader, stripping the trailing
// newline. Not using bufio.Scanner because we want control over the
// full line with no token-size limit.
func readLine(r io.Reader) (string, error) {
	br := bufio.NewReader(r)
	s, err := br.ReadString('\n')
	if err != nil && s == "" {
		return "", err
	}
	return strings.TrimRight(s, "\r\n"), nil
}

// openBrowser tries to open the given URL in the user's browser.
// Silently returns an error when no opener is available.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
