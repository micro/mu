package test

// A banner is drawn once, by the chrome.
//
// There are three — connect an agent, top up, verify before you post — and all
// three are prepended in app.renderForRequest, which every page goes through.
// /home also prepended the connect one itself, from back when it was the only
// page that carried it, so the screen somebody lands on after signing up drew
// it twice: the same words and the same button, one directly under the other.
//
// It is the failure mode of moving something into the chrome — the new call
// site is added and the old one is left, and nothing complains, because both
// halves work. So the rule is that there is one caller, and it is the shell.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// banners are the three, named as they are called.
var banners = regexp.MustCompile(`\bapp\.(ConnectBanner|CreditsBanner|VerifyBanner)\(`)

func TestOnlyTheChromeDrawsABanner(t *testing.T) {
	var found []string

	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if name := info.Name(); name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if banners.MatchString(line) {
				found = append(found, path+": "+strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// The three inside internal/app are the definitions and the one place they
	// are used; this scan is about everybody else. A qualified app.X call
	// cannot appear inside package app, so matching any at all is the failure.
	if len(found) > 0 {
		t.Errorf("a page draws a banner the chrome already draws, so it appears "+
			"twice:\n\t%s", strings.Join(found, "\n\t"))
	}
}
