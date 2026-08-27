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

	// `mu login https://their.host`, which is the form anybody would try first.
	//
	// Pointing the CLI at your own instance was `mu --url https://their.host
	// login` — a global flag before the subcommand, which is an order nobody
	// guesses and which the help did not show. On a binary that is meant to be
	// self-hosted, "how do I talk to my own instance" should not be the hard
	// part, and the answer is saved from here anyway.
	url := cfg.Server("")
	if len(args) > 0 {
		if at := strings.TrimSpace(args[0]); at != "" {
			if !strings.Contains(at, "://") {
				at = "https://" + at
			}
			url = strings.TrimRight(at, "/")
		}
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
	if err := client.Verify(); err != nil {
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
			// The instance that will actually be called, and what decided it.
			//
			// This printed the file's url, which is empty until somebody runs
			// login — so the one command for "what am I configured to do"
			// answered "url=" while every call went to micro.mu. Saying where
			// the value came from is the part that lets somebody change it.
			fmt.Printf("url=%s (%s)\n", cfg.Server(""), urlSource(cfg, file))
			if cfg.Token != "" {
				fmt.Println("token=***")
			} else {
				fmt.Println("token=  (run `mu login` to set one)")
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

// urlSource says what decided which instance to call, for `mu config get`.
//
// Reported rather than inferred by the reader: four things can set it and the
// value alone does not say which did, so somebody looking at an address they
// did not expect has no idea what to change.
func urlSource(cfg *ResolvedConfig, file *Config) string {
	at := cfg.Server("")
	switch {
	case os.Getenv("MU_URL") == at && at != "":
		return "MU_URL"
	case file != nil && file.URL == at && at != "":
		if p, err := configPath(); err == nil {
			return p
		}
		return "config file"
	case at != DefaultURL:
		return "--url"
	}
	return "default"
}
