// Package x402 is the toll on the door: how a caller with no account pays for
// one request and gets it.
//
// It is a protocol, not a capability. Nobody asks this instance for an x402 —
// they ask for a web search, and this is what happens in the middleware on the
// way to it: the price is looked up, a challenge is written, a signed payment
// is verified and settled through a facilitator, and a receipt goes back in a
// header. There is no page, no tool and no state of its own.
//
// So it lives under internal/ with the rest of the plumbing. It was in the
// wallet package, which put the till and the cash register in one directory
// and made "the wallet" mean two unrelated things: a key an agent spends, and
// the gate every priced request passes through. The gate does not need a key —
// verification is the facilitator's job, and this side only ever checks and
// settles what somebody else signed.
//
// The one thing it does not know is whether this instance can charge at all.
// That is Stripe keys or a receiving address, and the second half is only half
// — quota.Enabled is filled in from above by whoever holds both.
package x402

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"mu/internal/app"
	_ "mu/internal/env" // load ~/.env before x402 config is read at init
	"mu/internal/quota"
	"mu/internal/settings"
)

// x402 payment headers. Version 1 uses X-PAYMENT / X-PAYMENT-RESPONSE; version 2
// renamed them to PAYMENT-SIGNATURE / PAYMENT-RESPONSE. We accept either on the
// way in and emit both on the way out so any conformant client interoperates.
const (
	HeaderPaymentV1          = "X-PAYMENT"
	HeaderPaymentV2          = "PAYMENT-SIGNATURE"
	HeaderPaymentRespV1      = "X-PAYMENT-RESPONSE"
	HeaderPaymentRespV2      = "PAYMENT-RESPONSE"
	HeaderExtensionResponses = "EXTENSION-RESPONSES"
)

// x402 configuration from the environment. Defaults target Base mainnet via
// Coinbase's hosted facilitator; set X402_FACILITATOR_URL to the open
// (testnet) facilitator to certify without real funds.
var (
	x402FacilitatorURL = envOr("X402_FACILITATOR_URL", "https://api.cdp.coinbase.com/platform/v2/x402")
)

// payTo is the address payments are made to.
//
// A function, and read through settings, which is the whole of a bug worth
// stating plainly. It was a package variable initialised from os.Getenv, so it
// was fixed at process start from the environment alone: /admin/config offered
// an X402_PAY_TO box, saved what was typed into it, showed it as "saved here",
// and every 402 challenge went on naming whatever the environment said — or
// nothing, which is Enabled() returning false and payments simply not being
// offered.
//
// That is the worst place in this repository for that bug to have been, because
// the setting is where the money goes.
func payTo() string { return strings.TrimSpace(settings.Get("X402_PAY_TO")) }

// The advertised (network, version) pair.
//
// CDP registers specific pairs and settles the "exact" scheme as base+v1 or
// eip155:8453+v2. The default is the v2 pair: v1 works and settles perfectly
// well, but the discovery index carries only v2 entries, so a v1 server is
// payable and unfindable.
//
// Read live rather than fixed at init. These decide what every payer is told to
// sign, so a wrong pair stops paid calls working — and as os.Getenv the fix was
// an environment edit and a restart. Through settings they change from
// /admin/config and take effect on the next request, which is the difference
// between a minute of broken payments and however long a deploy takes.
// X402_NETWORK=base + X402_VERSION=1 goes back.
func Network() string {
	if v := strings.TrimSpace(settings.Get("X402_NETWORK")); v != "" {
		return v
	}
	return "eip155:8453"
}

func x402Ver() int {
	if v := strings.TrimSpace(settings.Get("X402_VERSION")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 2
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// NormalizeNetwork accepts either CAIP-2 ids (eip155:8453) or the short v1
// names (base) and returns the CAIP-2 id, which the CDP facilitator uses.
func NormalizeNetwork(n string) string {
	switch strings.ToLower(strings.TrimSpace(n)) {
	case "base", "eip155:8453":
		return "eip155:8453"
	case "base-sepolia", "eip155:84532":
		return "eip155:84532"
	default:
		return n
	}
}

// x402Asset is an ERC-20 accepted for payment on a given network. Name and
// Version are the token's EIP-712 domain parameters, echoed in a requirement's
// "extra" so the paying client can build the transfer-authorization signature.
type x402Asset struct {
	Symbol   string
	Address  string
	Decimals int
	Name     string
	Version  string
}

// x402AssetsByNetwork maps a network to its known stablecoins.
var x402AssetsByNetwork = map[string]map[string]x402Asset{
	"eip155:8453": { // Base mainnet
		"USDC": {"USDC", "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", 6, "USD Coin", "2"},
		"EURC": {"EURC", "0x60a3E35Cc302bFA44Cb288Bc5a4F316Fdb1adb42", 6, "EURC", "2"},
	},
	"eip155:84532": { // Base Sepolia
		"USDC": {"USDC", "0x036CbD53842c5426634e7929541eC2318f3dCF7e", 6, "USDC", "2"},
	},
}

// acceptedAssets returns the assets to advertise, honouring X402_ASSETS
// (comma-separated symbols) and defaulting to USDC only.
func acceptedAssets() []x402Asset {
	known := x402AssetsByNetwork[NormalizeNetwork(Network())]
	if known == nil {
		return nil
	}
	var out []x402Asset
	if list := strings.TrimSpace(os.Getenv("X402_ASSETS")); list != "" {
		for _, sym := range strings.Split(list, ",") {
			if a, ok := known[strings.ToUpper(strings.TrimSpace(sym))]; ok {
				out = append(out, a)
			}
		}
	}
	if len(out) == 0 {
		if a, ok := known["USDC"]; ok {
			out = append(out, a)
		}
	}
	return out
}

// Enabled reports whether x402 payments are configured.
func Enabled() bool { return payTo() != "" }

// x402 free trial — first N calls per wallet address are free. Tracked in
// memory (resets on restart, which is acceptable for a trial).
var (
	x402TrialLimit = 10
	x402TrialUsage = map[string]int{}
)

// UseTrialCall records a free trial call, returning false when exhausted.
func UseTrialCall(walletAddr string) bool {
	if walletAddr == "" || x402TrialUsage[walletAddr] >= x402TrialLimit {
		return false
	}
	x402TrialUsage[walletAddr]++
	app.Log("x402", "Free trial call %d/%d for %s", x402TrialUsage[walletAddr], x402TrialLimit, walletAddr)
	return true
}

// PaymentRequirements is a single accepted way to pay, matching the x402
// "exact" scheme. Amounts are in the asset's atomic units.
//
// Two shapes, one struct. v2 renamed maxAmountRequired to amount and moved
// resource, description and mimeType out of every requirement into one
// top-level resource object — the same facts, said once instead of per asset.
// Both sets are tagged omitempty and only the ones belonging to the advertised
// version are filled, so a v2 body carries no v1 fields and vice versa.
type PaymentRequirements struct {
	Scheme  string `json:"scheme"`
	Network string `json:"network"`

	// v2
	Amount string `json:"amount,omitempty"`

	// v1
	MaxAmountRequired string `json:"maxAmountRequired,omitempty"`
	Resource          string `json:"resource,omitempty"`
	Description       string `json:"description,omitempty"`
	MimeType          string `json:"mimeType,omitempty"`

	PayTo             string            `json:"payTo"`
	MaxTimeoutSeconds int               `json:"maxTimeoutSeconds"`
	Asset             string            `json:"asset"`
	Extra             map[string]string `json:"extra,omitempty"`
}

// AmountAtomic is what this requirement asks for, whichever version named it.
// Anything that signs or bounds a payment goes through here rather than reading
// a field, so a version flip cannot silently produce a zero amount.
func (r PaymentRequirements) AmountAtomic() string {
	if s := strings.TrimSpace(r.Amount); s != "" {
		return s
	}
	return strings.TrimSpace(r.MaxAmountRequired)
}

// SettleResponse is the facilitator's settlement result.
type SettleResponse struct {
	Success            bool   `json:"success"`
	Transaction        string `json:"transaction,omitempty"`
	Network            string `json:"network,omitempty"`
	Payer              string `json:"payer,omitempty"`
	ErrorReason        string `json:"errorReason,omitempty"`
	Message            string `json:"message,omitempty"`
	ExtensionResponses string `json:"-"`
}

// creditsToAtomic converts a credit cost (Mu treats 1 credit ≈ 1 US cent) into
// the token's atomic units: cents * 10^(decimals-2). For 6-decimal USDC, 5
// credits ($0.05) → "50000".
func creditsToAtomic(credits, decimals int) string {
	if credits < 1 {
		credits = 1
	}
	mult := 1
	for i := 0; i < decimals-2; i++ {
		mult *= 10
	}
	return strconv.Itoa(credits * mult)
}

// BuildPaymentRequirements creates the accepted-payment list for an operation —
// one entry per accepted asset; the paying agent picks one. Nil for a free
// operation: there is nothing to charge, so there is nothing to accept.
//
// This used to floor the cost at one credit, which quietly turned every free
// tool into a paid one at the door. The four tools deliberately priced at zero
// answered an anonymous caller with a demand for USDC on Base — so the free
// tier existed in the price list and was unreachable in practice, which is the
// opposite of what /mcp being a public endpoint is for.
func BuildPaymentRequirements(operation, resource string) []PaymentRequirements {
	cost := quota.OperationCost(operation)
	if cost < 1 {
		return nil
	}
	var reqs []PaymentRequirements
	for _, a := range acceptedAssets() {
		r := PaymentRequirements{
			Scheme:            "exact",
			Network:           Network(),
			PayTo:             payTo(),
			MaxTimeoutSeconds: 60,
			Asset:             a.Address,
			Extra:             map[string]string{"name": a.Name, "version": a.Version},
		}
		amount := creditsToAtomic(cost, a.Decimals)
		if x402Ver() >= 2 {
			r.Amount = amount
		} else {
			r.MaxAmountRequired = amount
			r.Resource = resource
			r.Description = "Access to " + operation
			r.MimeType = "application/json"
		}
		reqs = append(reqs, r)
	}
	return reqs
}

// WritePaymentRequired sends the standard x402 402 challenge: an HTTP 402 whose
// JSON body carries the accepted payment requirements. No custom headers — the
// body is the contract, as every x402 client expects.
//
// The error string used to be "X-PAYMENT header required", which is true and
// useless. It is the first thing a developer sees when they try a metered tool
// against the endpoint, and it named a header they have never heard of, in the
// vocabulary of a payment rail this instance does not lead with. An x402 client
// reads accepts and ignores this line; a person reads this line and nothing
// else, so it should tell them what they can actually do.
//
// Returns false when the operation is free, having written nothing: the caller
// must then let the request through rather than block it. A gate that cannot
// say "there is nothing to pay" can only say "pay", which is how free tools
// came to be paywalled.
// extensions, when non-nil, are published alongside accepts — today that is
// the Bazaar discovery listing, which is how a facilitator learns this endpoint
// exists at the moment it is asked what it charges.
// reason, when non-empty, replaces the default wording. A caller with no
// account is told to sign in or pay; one that is signed in and out of credits
// needs different words, because "sign in" is not advice it can act on.
func WritePaymentRequired(w http.ResponseWriter, operation, resource string, extensions map[string]any, reason string) bool {
	reqs := BuildPaymentRequirements(operation, resource)
	if len(reqs) == 0 {
		return false
	}
	if reason == "" {
		reason = "Payment required. Choose an entry from accepts, submit payment, and retry."
	}
	if extensions == nil {
		extensions = BazaarExtensions(operation, resource)
	}
	body := map[string]any{
		"x402Version": x402Ver(),
		"error":       reason,
		"accepts":     reqs,
	}
	// v2 says what is being bought once, at the top, rather than repeating it in
	// every requirement. serviceName and tags are read by the discovery index —
	// they are how a listing gets a name and turns up in a search rather than
	// only in a full enumeration.
	if x402Ver() >= 2 {
		body["resource"] = map[string]any{
			"url":         resource,
			"description": "Access to " + operation,
			"mimeType":    "application/json",
			"serviceName": serviceName(resource),
			"tags":        []string{"mcp", "agent-tools"},
		}
	}
	if len(extensions) > 0 {
		body["extensions"] = extensions
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPaymentRequired)
	_ = json.NewEncoder(w).Encode(body)
	return true
}

// HasPayment reports whether a request carries an x402 payment header.
func HasPayment(r *http.Request) bool {
	return r.Header.Get(HeaderPaymentV1) != "" || r.Header.Get(HeaderPaymentV2) != ""
}

func paymentHeader(r *http.Request) string {
	if v := r.Header.Get(HeaderPaymentV1); v != "" {
		return v
	}
	return r.Header.Get(HeaderPaymentV2)
}

// VerifyAndSettle decodes the payment header, verifies it with the facilitator
// and, if valid, settles it. On success the settlement is stashed on the
// request context (see SettleHolder) so the response layer can emit the
// X-PAYMENT-RESPONSE header. Returns the settlement or an error.
func VerifyAndSettle(r *http.Request, operation, resource string) (*SettleResponse, error) {
	v, err := Verify(r, operation, resource)
	if err != nil {
		return nil, err
	}
	return v.Settle()
}

// Verified is a payment the facilitator has checked and not yet moved.
//
// The two halves of a payment are separate calls in the protocol for a reason,
// and this instance was collapsing them: it verified and settled at the door,
// before the tool ran and therefore before anything had looked at the
// arguments. So a request that failed afterwards — a mistyped number, a
// provider down, a panic — was a charge with no answer. That is the one thing a
// pay-per-call endpoint must never do, and it was the default path.
//
// Verify says the money is good and moves none of it. Settle moves it, once
// there is something to hand back.
type Verified struct {
	payload map[string]any
	req     *PaymentRequirements
}

// Verify checks a payment without moving any money.
//
// Everything that can refuse a caller happens here: a malformed header, a
// payment for the wrong thing, an authorization the facilitator will not
// honour. What is left after this returns is a promise that settling will work,
// which is what makes it safe to do the work first.
func Verify(r *http.Request, operation, resource string) (*Verified, error) {
	hdr := paymentHeader(r)
	if hdr == "" {
		return nil, fmt.Errorf("no payment header")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(hdr))
	if err != nil {
		if raw, err = base64.RawURLEncoding.DecodeString(strings.TrimSpace(hdr)); err != nil {
			return nil, fmt.Errorf("invalid payment header encoding: %w", err)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("invalid payment payload: %w", err)
	}

	req := matchRequirement(BuildPaymentRequirements(operation, resource), payload)
	if req == nil {
		return nil, fmt.Errorf("no matching payment requirement for the presented payment")
	}
	if err := verifyRequirement(payload, req); err != nil {
		return nil, err
	}

	v := &Verified{payload: payload, req: req}
	// Hand it to the writer, which settles once it knows the work succeeded.
	if h, ok := r.Context().Value(X402SettleKey).(*SettleHolder); ok && h != nil {
		h.Verified = v
	}
	return v, nil
}

// Settle moves the money for a payment that has already been verified.
func (v *Verified) Settle() (*SettleResponse, error) {
	if v == nil {
		return nil, fmt.Errorf("nothing verified to settle")
	}
	return settleVerified(v.payload, v.req)
}

// Payer is the address that signed this payment.
//
// Read from the authorization the facilitator has just verified, so it is
// authenticated in the way that matters — only the holder of that key could
// have produced it — and it is available before settlement, which is what lets
// an account-scoped tool know who is calling while the money is still unmoved.
func (v *Verified) Payer() string {
	if v == nil {
		return ""
	}
	inner, _ := v.payload["payload"].(map[string]any)
	auth, _ := inner["authorization"].(map[string]any)
	from, _ := auth["from"].(string)
	return strings.ToLower(strings.TrimSpace(from))
}

// settlePayload verifies then settles a decoded payment payload against the
// requirements for operation (the server-side x402 path, VerifyAndSettle).
func settlePayload(payload map[string]any, operation, resource string) (*SettleResponse, error) {
	req := matchRequirement(BuildPaymentRequirements(operation, resource), payload)
	if req == nil {
		return nil, fmt.Errorf("no matching payment requirement for the presented payment")
	}
	return settleRequirement(payload, req)
}

// verifyRequirement asks the facilitator whether a payment would succeed.
//
// No money moves. This is the half that can say no.
func verifyRequirement(payload map[string]any, req *PaymentRequirements) error {
	body := map[string]any{"x402Version": x402Ver(), "paymentPayload": payload, "paymentRequirements": req}

	vres, _, err := facilitatorPost("/verify", body)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	var verify struct {
		IsValid        bool   `json:"isValid"`
		Valid          bool   `json:"valid"`
		InvalidReason  string `json:"invalidReason"`
		InvalidMessage string `json:"invalidMessage"`
	}
	_ = json.Unmarshal(vres, &verify)
	if !verify.IsValid && !verify.Valid {
		return fmt.Errorf("payment invalid: %s", firstNonEmpty(verify.InvalidMessage, verify.InvalidReason, "rejected by facilitator"))
	}
	return nil
}

// settleVerified moves the money for a payment that verifyRequirement passed.
func settleVerified(payload map[string]any, req *PaymentRequirements) (*SettleResponse, error) {
	body := map[string]any{"x402Version": x402Ver(), "paymentPayload": payload, "paymentRequirements": req}

	sres, headers, err := facilitatorPost("/settle", body)
	if err != nil {
		return nil, fmt.Errorf("settle: %w", err)
	}
	var settle SettleResponse
	_ = json.Unmarshal(sres, &settle)
	if !settle.Success {
		return nil, fmt.Errorf("settlement failed: %s", firstNonEmpty(settle.Message, settle.ErrorReason, "unknown"))
	}
	settle.ExtensionResponses = strings.TrimSpace(headers.Get(HeaderExtensionResponses))
	app.Log("x402", "settled %s: tx=%s payer=%s", req.Resource, settle.Transaction, settle.Payer)
	return &settle, nil
}

// settleRequirement verifies then settles in one step, for a caller that has
// nothing to do in between — the outbound payer, where this instance is the one
// paying somebody else.
func settleRequirement(payload map[string]any, req *PaymentRequirements) (*SettleResponse, error) {
	if err := verifyRequirement(payload, req); err != nil {
		return nil, err
	}
	return settleVerified(payload, req)
}

// matchRequirement picks the advertised requirement the payment was made
// against, by network (and asset when the payload names one). Falls back to the
// first requirement, which is correct for the single-asset default.
func matchRequirement(reqs []PaymentRequirements, payload map[string]any) *PaymentRequirements {
	if len(reqs) == 0 {
		return nil
	}
	net, _ := payload["network"].(string)
	net = NormalizeNetwork(net)
	asset := payloadAsset(payload)
	for i := range reqs {
		if net != "" && NormalizeNetwork(reqs[i].Network) != net {
			continue
		}
		if asset != "" && !strings.EqualFold(reqs[i].Asset, asset) {
			continue
		}
		return &reqs[i]
	}
	return &reqs[0]
}

// payloadAsset best-effort extracts the token address from a payment payload,
// tolerating the v1/v2 nesting differences.
func payloadAsset(payload map[string]any) string {
	if a, ok := payload["asset"].(string); ok && a != "" {
		return a
	}
	if p, ok := payload["payload"].(map[string]any); ok {
		if a, ok := p["asset"].(string); ok {
			return a
		}
	}
	return ""
}

// facilitatorPost POSTs JSON to a facilitator endpoint, attaching a CDP Bearer
// JWT when CDP credentials are configured (required by the Coinbase-hosted
// facilitator; ignored by the open one).
func facilitatorPost(path string, body map[string]any) ([]byte, http.Header, error) {
	endpoint := strings.TrimRight(x402FacilitatorURL, "/") + path
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if cdpConfigured() {
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, nil, err
		}
		bearer, err := cdpBearer(http.MethodPost, u.Host, u.Path)
		if err != nil {
			return nil, nil, fmt.Errorf("cdp auth: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, &facilitatorError{Path: path, Status: resp.StatusCode, Body: data}
	}
	return data, resp.Header.Clone(), nil
}

// facilitatorError is a non-200 from the facilitator, carrying the body so it
// can be turned into something the caller can act on.
//
// It used to be a fmt.Errorf that pasted the raw JSON into the message, and
// that string is what an agent developer sees on their first attempt — the
// commonest first attempt being a wallet with no USDC in it, which comes back
// as `execution reverted`. Nothing in those words says "your wallet is empty",
// so the one error most likely to be hit was the one hardest to act on.
type facilitatorError struct {
	Path   string
	Status int
	Body   []byte
}

func (e *facilitatorError) Error() string {
	var f struct {
		InvalidReason  string `json:"invalidReason"`
		InvalidMessage string `json:"invalidMessage"`
		Message        string `json:"message"`
		ErrorReason    string `json:"errorReason"`
		Payer          string `json:"payer"`
	}
	_ = json.Unmarshal(e.Body, &f)
	detail := firstNonEmpty(f.InvalidMessage, f.Message, f.ErrorReason, f.InvalidReason)

	// A reverted transfer means the token contract refused it. The payer's
	// balance is the reason in almost every case, and it is the only one the
	// payer can do anything about, so it leads.
	if strings.Contains(strings.ToLower(detail), "execution reverted") {
		who := "the paying wallet"
		if f.Payer != "" {
			who = f.Payer
		}
		return fmt.Sprintf("the payment could not be taken — %s does not hold enough USDC on this network", who)
	}
	if detail != "" {
		return detail
	}
	return fmt.Sprintf("facilitator %s returned %d: %s", e.Path, e.Status, strings.TrimSpace(string(e.Body)))
}

// Status returns a human-readable diagnostic of the x402 configuration
// and, when CDP credentials are present, the facilitator's advertised support —
// so an operator can certify auth on the box without exposing the secret.
func Status() string {
	var b strings.Builder
	fmt.Fprintf(&b, "enabled:       %v\n", Enabled())
	fmt.Fprintf(&b, "pay-to:        %s\n", firstNonEmpty(payTo(), "(X402_PAY_TO not set)"))
	fmt.Fprintf(&b, "facilitator:   %s\n", x402FacilitatorURL)
	fmt.Fprintf(&b, "network:       %s\n", Network())
	fmt.Fprintf(&b, "version:       %d\n", x402Ver())
	var syms []string
	for _, a := range acceptedAssets() {
		syms = append(syms, a.Symbol)
	}
	fmt.Fprintf(&b, "assets:        %s\n", firstNonEmpty(strings.Join(syms, ","), "(none for this network)"))
	fmt.Fprintf(&b, "cdp auth:      %v\n", cdpConfigured())

	if !cdpConfigured() {
		b.WriteString("\nNo CDP credentials (CDP_API_KEY_ID / CDP_API_KEY_SECRET). The open\nfacilitator only settles testnets; set CDP creds for Base mainnet.\n")
		return b.String()
	}

	endpoint := strings.TrimRight(x402FacilitatorURL, "/") + "/supported"
	u, err := url.Parse(endpoint)
	if err != nil {
		fmt.Fprintf(&b, "\ncdp probe:     bad facilitator URL: %v\n", err)
		return b.String()
	}
	bearer, err := cdpBearer(http.MethodGet, u.Host, u.Path)
	if err != nil {
		fmt.Fprintf(&b, "\ncdp probe:     JWT build failed: %v\n", err)
		return b.String()
	}
	req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		fmt.Fprintf(&b, "\ncdp probe:     request failed: %v\n", err)
		return b.String()
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(&b, "\ncdp probe:     HTTP %d — auth NOT working: %s\n", resp.StatusCode, strings.TrimSpace(string(data)))
		return b.String()
	}
	fmt.Fprintf(&b, "\ncdp probe:     OK — auth working. Supported schemes/networks:\n%s\n", strings.TrimSpace(string(data)))
	if !strings.Contains(string(data), Network()) {
		fmt.Fprintf(&b, "\nWARNING: configured network %s not in the supported list above.\n", Network())
	}
	return b.String()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// ── settlement response header plumbing ─────────────────────────────────────

type x402ContextKeyType struct{}

// X402ContextKey marks requests whose payment should be verified+settled.
var X402ContextKey = x402ContextKeyType{}

type x402SettleKeyType struct{}

// X402SettleKey holds a *SettleHolder that Verify fills once a payment checks
// out, and that the response writer settles from once the work has succeeded.
var X402SettleKey = x402SettleKeyType{}

// SettleHolder carries a verified-but-unmoved payment out of the gate (which
// runs deep inside the tool dispatch) to the response writer, which is the only
// thing that knows whether there is an answer to charge for.
type SettleHolder struct {
	Verified *Verified
	Resp     *SettleResponse
}

// settleWriter holds the response back until it knows whether the work
// succeeded, settles the payment if it did, and only then writes.
//
// Holding it back is the point. The receipt is an HTTP header, headers go
// before the body, and whether a JSON-RPC call succeeded is in the body — so
// deciding on the way past is impossible and the response has to be complete
// before anything is charged. These are single JSON documents on one endpoint,
// so there is nothing here worth streaming, and http.Flusher is deliberately
// not implemented: a handler that could flush could commit a status line before
// the money question was asked.
type settleWriter struct {
	http.ResponseWriter
	holder *SettleHolder
	code   int
	body   bytes.Buffer
}

// NewSettleWriter wraps w so the payment settles only if the work succeeds.
func NewSettleWriter(w http.ResponseWriter, h *SettleHolder) http.ResponseWriter {
	return &settleWriter{ResponseWriter: w, holder: h}
}

func (s *settleWriter) WriteHeader(code int) {
	if s.code == 0 {
		s.code = code
	}
}

func (s *settleWriter) Write(b []byte) (int, error) {
	if s.code == 0 {
		s.code = http.StatusOK
	}
	return s.body.Write(b)
}

// Finish settles if there is something to charge for, then sends the response.
//
// Called by the server once the handler has returned. A caller that forgets is
// a caller whose response never arrives, which is the right way round for this
// mistake to fail.
func Finish(w http.ResponseWriter) {
	s, ok := w.(*settleWriter)
	if !ok {
		return
	}
	s.finish()
}

func (s *settleWriter) finish() {
	if s.code == 0 {
		s.code = http.StatusOK
	}
	body := s.body.Bytes()

	if s.holder != nil && s.holder.Verified != nil && succeeded(s.code, body) {
		resp, err := s.holder.Verified.Settle()
		switch {
		case err != nil:
			// The work is done and the money did not move. The caller gets
			// their answer for nothing, which is the right way to be wrong:
			// the invariant worth keeping is that nobody is ever charged
			// without an answer, and refusing here would break it in the
			// other direction — they would have paid nothing and received
			// nothing, having done nothing wrong.
			app.Alert("x402", "work succeeded and settlement failed, "+
				"so this answer was given away: %v", err)
		default:
			s.holder.Resp = resp
			b, _ := json.Marshal(resp)
			enc := base64.StdEncoding.EncodeToString(b)
			s.Header().Set(HeaderPaymentRespV1, enc)
			s.Header().Set(HeaderPaymentRespV2, enc)
			if resp.ExtensionResponses != "" {
				s.Header().Set(HeaderExtensionResponses, resp.ExtensionResponses)
			}
		}
	}

	s.ResponseWriter.WriteHeader(s.code)
	if len(body) > 0 {
		s.ResponseWriter.Write(body) //nolint:errcheck
	}
}

// succeeded reports whether this response is something worth charging for.
//
// The HTTP status is necessary and not sufficient: MCP answers a failed tool
// call with 200 and a JSON-RPC error, which is exactly the case that was being
// charged for — a mistyped argument cost real money and returned a complaint
// about unmarshalling.
func succeeded(code int, body []byte) bool {
	if code < 200 || code >= 300 {
		return false
	}
	var rpc struct {
		Error  json.RawMessage `json:"error"`
		Result *struct {
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &rpc); err != nil {
		// Not JSON-RPC. A 2xx from anything else is a success by the only
		// measure available, and refusing to charge for every non-JSON answer
		// would be a worse error than the one being fixed.
		return true
	}
	if len(rpc.Error) > 0 && string(rpc.Error) != "null" {
		return false
	}
	if rpc.Result != nil && rpc.Result.IsError {
		return false
	}
	return true
}

// PayerFrom returns the wallet address that paid for this request, or "" if
// nothing was paid.
//
// The address comes from a settled payment — the facilitator verified the
// signature and moved the funds — so it is authenticated in the way that
// matters: only the holder of that key could have produced it. That makes it
// usable as an identity, which is what lets an agent with no account reach a
// service that holds per-caller data.
//
// It is deliberately not read from the X-Wallet-Address header. That header
// exists for the free-trial counter and is unauthenticated: anyone can claim
// any address. Using it for identity would let one caller read another's
// records by typing their address.
func PayerFrom(ctx context.Context) string {
	h, ok := ctx.Value(X402SettleKey).(*SettleHolder)
	if !ok || h == nil {
		return ""
	}
	// The verified payment first, because that is what exists while the tool is
	// running. This used to read the settlement, which was fine when settling
	// happened at the door and became a silent hole the moment it moved to the
	// end: an account-scoped tool reached by paying would have asked who was
	// calling, got nothing, and refused a call that had been paid for.
	//
	// It is no weaker. The address comes from the authorization the facilitator
	// has just checked the signature on — only the holder of that key could
	// have produced it — and that is true before the money moves as much as
	// after.
	if h.Verified != nil {
		if addr := h.Verified.Payer(); addr != "" {
			return addr
		}
	}
	if h.Resp == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(h.Resp.Payer))
}

// serviceName is what this instance calls itself in a discovery listing.
//
// The public resource is the authoritative identity for a request. That matters
// when one Mu instance is exposed through more than one public surface: the
// x402 challenge should name the host the caller is actually paying, not leak
// the instance's configured mail/application domain. Configuration remains the
// fallback for callers that provide a non-URL resource.
func serviceName(resource string) string {
	if u, err := url.Parse(strings.TrimSpace(resource)); err == nil && u.Host != "" {
		return u.Host
	}
	if u := strings.TrimSpace(settings.Get("MU_DOMAIN")); u != "" {
		return u
	}
	if u := strings.TrimSpace(settings.Get("APP_URL")); u != "" {
		return strings.TrimPrefix(strings.TrimPrefix(strings.TrimRight(u, "/"), "https://"), "http://")
	}
	return "mu"
}

// TopUpRequirement is a payment of an arbitrary number of credits to this
// instance, for somebody buying credits with USDC rather than a card.
//
// BuildPaymentRequirements cannot answer this. It prices a named operation by
// looking it up in quota, which is right for a tool call and has no meaning for
// a top-up: the amount is whatever somebody chose to convert. Same shape, same
// asset list, same payTo — only the amount comes from the caller.
//
// One asset rather than a list, because there is nobody to choose. A 402
// challenge offers several and lets the payer pick; here this instance is both
// sides, so it takes the first accepted asset and signs against that.
//
// Nil when x402 is not configured, or when there is no accepted asset to pay
// in — the caller must treat that as "this instance cannot be paid this way"
// rather than as a failure to convert.
func TopUpRequirement(credits int) *PaymentRequirements {
	if !Enabled() || credits < 1 {
		return nil
	}
	assets := acceptedAssets()
	if len(assets) == 0 {
		return nil
	}
	a := assets[0]

	r := PaymentRequirements{
		Scheme:            "exact",
		Network:           Network(),
		PayTo:             payTo(),
		MaxTimeoutSeconds: 120,
		Asset:             a.Address,
		Extra:             map[string]string{"name": a.Name, "version": a.Version},
	}
	amount := creditsToAtomic(credits, a.Decimals)
	if x402Ver() >= 2 {
		r.Amount = amount
	} else {
		r.MaxAmountRequired = amount
		r.Resource = "credits"
		r.Description = "Credits on this instance"
		r.MimeType = "application/json"
	}
	return &r
}

// SettleSigned verifies and settles a payment this instance signed itself.
//
// The rest of this file settles payments that arrived: an X-PAYMENT header on
// an inbound request, matched against the requirements for the operation being
// called. A top-up has no inbound request — this instance holds the key, signs
// the authorisation and hands it to the facilitator — so it needs the same two
// facilitator calls without the HTTP request in front of them.
//
// hdr is the base64 header value SignX402Payment returns, so the wallet keeps
// producing exactly one thing and this decodes it the same way an inbound
// payment is decoded.
func SettleSigned(hdr string, req *PaymentRequirements) (*SettleResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("no payment requirement")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(hdr))
	if err != nil {
		return nil, fmt.Errorf("payment payload is not base64: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("payment payload is not JSON: %w", err)
	}
	return settleRequirement(payload, req)
}
