package test

// One system of record for money.
//
// A price on an endpoint used to be a label. Four different places could charge
// for a call — a function both doors happened to call, a page handler, a
// hardcoded list of paths in HTTP middleware, or nothing at all — and which one
// a service used was historical accident. The result was measurable from
// outside: on the live instance two web_search calls against an ordinary
// account moved the balance from 75 to 75, while images_generate in the same
// minute moved it from 75 to 15.
//
// Now every service call goes through one gateway (internal/service/gateway.go)
// which charges the operation the endpoint declares. That leaves exactly two
// legitimate arrangements, and this file exists to stop a third appearing:
//
//   - the endpoint declares a Cost, and the gateway charges it. The service
//     must not also charge, or every call is billed twice.
//   - the endpoint declares no Cost because its price is not a constant — mail
//     costs one thing locally and another externally, sms is priced per
//     segment, whatsapp charges only when a message opens a conversation — and
//     the service charges for itself.
//
// What is not allowed is both, and what is not allowed is neither.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"mu/internal/service"
)

// consumesOps returns every quota operation a service package debits itself.
func consumesOps(t *testing.T, pkgDir string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	// app.Charge is the page door's wrapper around the same debit, so it counts.
	re := regexp.MustCompile(`(?:Consume(?:Quota|With)|app\.Charge)\([^,]+,\s*quota\.(\w+)`)
	filepath.Walk(pkgDir, func(path string, info os.FileInfo, err error) error { //nolint:errcheck
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, m := range re.FindAllSubmatch(b, -1) {
			out[string(m[1])] = true
		}
		return nil
	})
	return out
}

// opConstants maps quota's Go constant names to the operation ids they hold, so
// a Spec saying quota.OpWebSearch and a service saying the same can be compared
// with what quota.json prices.
func opConstants(t *testing.T) map[string]string {
	t.Helper()
	b, err := os.ReadFile(at("internal", "quota", "quota.go"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, m := range regexp.MustCompile(`(Op\w+)\s*=\s*"([^"]+)"`).FindAllStringSubmatch(string(b), -1) {
		out[m[1]] = m[2]
	}
	return out
}

// price is one line of quota.json, read from the file rather than from the
// package, so this test sees what an operator sees.
type price struct {
	op   string
	cost int
}

func quotaPrices(t *testing.T) []price {
	t.Helper()
	b, err := os.ReadFile(at("quota.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Operations []struct {
			Op   string `json:"op"`
			Cost int    `json:"cost"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	out := make([]price, 0, len(f.Operations))
	for _, o := range f.Operations {
		out = append(out, price{op: o.Op, cost: o.Cost})
	}
	return out
}

// pageCharged is the ledger of what the gateway does not yet cover.
//
// A page reaches past the endpoint into a service's own functions, so it cannot
// be charged by a wrapper around the endpoint. Until pages call endpoints these
// services legitimately charge in two places — the gateway for the tool door,
// their own handler for the page — and this is the list of them.
//
// It is debt, written down. Every entry that goes is a page moved onto its
// endpoint, and when the map is empty the transition is finished and both this
// variable and the exception it grants should be deleted.
var pageCharged = map[string]bool{
	"social_post": true, "social_reply": true, "social_search": true,
	"blog_create": true, "blog_comment": true,
	"app_create": true, "app_build": true, "app_edit": true,
	"stream_post":   true,
	"places_search": true, "places_nearby": true,
	"image_generate": true,
	"weather_pollen": true, "weather_forecast": true,
	"web_search": true, "web_fetch": true,
	"news_search": true,
	// /mail's Handler charges for both, for the same reason: the page sends
	// mail without going through the endpoint.
	"mail_send": true, "external_email": true,
}

// viaHook is charged by the agent and the app runtime through ChargeQuota,
// which is a function variable filled in at boot rather than a call this can
// see. These are the model's own operations rather than a service's, so no
// endpoint declares them and no service debits them.
var viaHook = map[string]bool{
	"agent_query": true, "agent_query_premium": true, "chat_query": true,
}

// TestNoOperationIsChargedTwice — a service must not debit an operation its own
// endpoint already declares, because the gateway charges that one.
func TestNoOperationIsChargedTwice(t *testing.T) {
	consts := opConstants(t)
	byID := map[string]string{}
	for name, id := range consts {
		byID[id] = name
	}

	for _, spec := range allSpecs() {
		dir := at("service", spec.Name)
		if _, err := os.Stat(dir); err != nil {
			continue // a service whose package is not named after it
		}
		selfCharged := consumesOps(t, dir)

		for method, ep := range spec.Endpoints {
			if ep.Cost == "" {
				continue
			}
			name := byID[ep.Cost]
			if name == "" {
				continue
			}
			if selfCharged[name] && !pageCharged[ep.Cost] {
				t.Errorf("%s.%s declares Cost %s and %s/ also debits it — "+
					"the gateway charges declared costs, so this call is billed twice",
					spec.Name, method, ep.Cost, spec.Name)
			}
		}
	}
}

// TestEveryPricedOperationIsChargedSomewhere — the failure this whole thing was
// built for. An operation with a price in quota.json that nothing debits is
// free in practice, and nothing anywhere says so.
func TestEveryPricedOperationIsChargedSomewhere(t *testing.T) {
	consts := opConstants(t)

	// Declared on an endpoint: the gateway charges it.
	declared := map[string]bool{}
	for _, spec := range allSpecs() {
		for _, ep := range spec.Endpoints {
			if ep.Cost != "" {
				declared[ep.Cost] = true
			}
		}
	}

	// Debited by a service itself: the variable-price arrangement.
	selfCharged := map[string]bool{}
	entries, err := os.ReadDir(at("service"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		for name := range consumesOps(t, at("service", e.Name())) {
			if id, ok := consts[name]; ok {
				selfCharged[id] = true
			}
		}
	}

	var free []string
	for _, p := range quotaPrices(t) {
		if p.cost == 0 {
			continue // priced at zero on purpose: it costs this instance nothing
		}
		if declared[p.op] || selfCharged[p.op] || pageCharged[p.op] || viaHook[p.op] {
			continue
		}
		free = append(free, p.op)
	}
	sort.Strings(free)
	if len(free) > 0 {
		t.Errorf("%d operation(s) have a price and nothing charges them, so they are "+
			"free in practice: %s", len(free), strings.Join(free, ", "))
	}
}

// TestTheGatewayIsTheOnlyThingWiredToCharge — the gateway is only a single
// system of record while it is the only thing holding the pen.
func TestTheGatewayIsTheOnlyThingWiredToCharge(t *testing.T) {
	if service.Gate.Allow != nil || service.Gate.Charge != nil {
		t.Fatal("the gate is already wired in a test binary, so this proves nothing")
	}
	// Every service that declares a cost must be reachable through the gateway,
	// which means it must be a registered Spec — the thing the wrapper is
	// attached to at Register.
	for _, spec := range allSpecs() {
		for method, ep := range spec.Endpoints {
			if ep.Cost == "" {
				continue
			}
			if spec.Operation(method) != ep.Cost {
				t.Errorf("%s.%s declares %s but the gateway would look up %q",
					spec.Name, method, ep.Cost, spec.Operation(method))
			}
		}
	}
}
