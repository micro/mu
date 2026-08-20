package api

// One door for a tool a model named.
//
// A tool the agent holds is a tool prompt injection holds, so two questions
// have to be asked before a model's chosen tool runs: may a caller with no
// account use it, and is it one of the destructive ones withheld from the model
// entirely. Both questions were asked by the callers. There were four execution
// sites across agent/ and agent/micro/, each with the guards written out above
// the call, and the arithmetic never held: the destructive check was present at
// one site and absent at the next for as long as agent/micro existed, and the
// residual guest allowlist was copied into two files that then drifted.
//
// The fix is not another test that the guards are near the calls. It is that
// ExecuteTool and ExecuteToolAs are not what an agent calls. RunPlanned and
// RunPlannedAs are, they ask both questions first, and a fifth execution site
// written next year gets the answers whether or not its author knew to.
//
// Not MCP. A client holding a token is a program somebody wrote, not a model
// choosing from a catalogue after reading a stranger's web page — it reaches
// ExecuteTool directly and gets the destructive tools with an annotation saying
// so, which is what the annotation is for.

import (
	"errors"
	"fmt"
	"net/http"

	"mu/internal/service"
)

// ErrRefused is returned when a tool a model named will not be run. Callers
// generally skip the step quietly rather than telling the model it failed —
// "unavailable" invites a retry, and this is not a transient condition.
var ErrRefused = errors.New("withheld from the model")

// publicToolExtras are the guest-usable tools with no service behind them, so
// service.GuestAllowedTool cannot answer for them. Anything service-backed is
// derived from whether that service is account-scoped.
//
// One copy. There were two, in agent/guest.go and agent/micro/execute.go, both
// carrying the comment about the two hand-written allowlists they had replaced.
var publicToolExtras = map[string]bool{
	"quran":         true,
	"quran_search":  true,
	"hadith":        true,
	"blog_read":     true,
	"social_search": true,
	"video_search":  true,
	"apps_run":      true,
}

// PublicTool reports whether a tool may run in a context with nothing private
// in it — a group channel, where the conversation is not one person's.
//
// It was GuestTool, for a caller with no account. There are none: every run
// belongs to an account now. What survives is the group case, which asks the
// same question about the same set of tools.
func PublicTool(name string) bool {
	return service.PublicTool(name) || publicToolExtras[name]
}

// AllowPlanned reports whether a tool a model named may be offered to it or run
// for it, and why not when it may not.
//
// It is exported because the listing side needs the same answer: a tool a model
// is never shown is a tool it is less likely to name, and the two must agree or
// the model spends its turns naming things that are then refused.
func AllowPlanned(name string, guest bool) error {
	if service.DestructiveTool(name) {
		return fmt.Errorf("%w: %s is destructive", ErrRefused, name)
	}
	if guest && !PublicTool(name) {
		return fmt.Errorf("%w: %s is not available in a group", ErrRefused, name)
	}
	return nil
}

// RunPlanned executes a tool a model named, on behalf of the HTTP caller.
func RunPlanned(r *http.Request, guest bool, name string, args map[string]any) (string, bool, error) {
	if err := AllowPlanned(name, guest); err != nil {
		return "", true, err
	}
	return ExecuteTool(r, name, args)
}

// RunPlannedAs executes a tool a model named, as an account.
func RunPlannedAs(accountID string, guest bool, name string, args map[string]any) (string, bool, error) {
	if err := AllowPlanned(name, guest); err != nil {
		return "", true, err
	}
	return ExecuteToolAs(accountID, name, args)
}
