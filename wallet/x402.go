package wallet

import (
	"bytes"
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
)

// x402 payment headers. Version 1 uses X-PAYMENT / X-PAYMENT-RESPONSE; version 2
// renamed them to PAYMENT-SIGNATURE / PAYMENT-RESPONSE. We accept either on the
// way in and emit both on the way out so any conformant client interoperates.
const (
	HeaderPaymentV1     = "X-PAYMENT"
	HeaderPaymentV2     = "PAYMENT-SIGNATURE"
	HeaderPaymentRespV1 = "X-PAYMENT-RESPONSE"
	HeaderPaymentRespV2 = "PAYMENT-RESPONSE"
)

// x402 configuration from the environment. Defaults target Base mainnet via
// Coinbase's hosted facilitator; set X402_FACILITATOR_URL to the open
// (testnet) facilitator to certify without real funds.
var (
	x402PayTo          = strings.TrimSpace(os.Getenv("X402_PAY_TO"))          // receiving address
	x402FacilitatorURL = envOr("X402_FACILITATOR_URL", "https://api.cdp.coinbase.com/platform/v2/x402")
	x402NetworkID      = normalizeNetwork(envOr("X402_NETWORK", "eip155:8453")) // CAIP-2, Base mainnet
	x402Version        = envIntOr("X402_VERSION", 1)                            // advertised protocol version
)

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

// normalizeNetwork accepts either CAIP-2 ids (eip155:8453) or the short v1
// names (base) and returns the CAIP-2 id, which the CDP facilitator uses.
func normalizeNetwork(n string) string {
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
	known := x402AssetsByNetwork[x402NetworkID]
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

// X402Enabled reports whether x402 payments are configured.
func X402Enabled() bool { return x402PayTo != "" }

// x402 free trial — first N calls per wallet address are free. Tracked in
// memory (resets on restart, which is acceptable for a trial).
var (
	x402TrialLimit = 10
	x402TrialUsage = map[string]int{}
)

// X402TrialRemaining returns how many free calls the address has left.
func X402TrialRemaining(walletAddr string) int {
	if walletAddr == "" {
		return 0
	}
	if r := x402TrialLimit - x402TrialUsage[walletAddr]; r > 0 {
		return r
	}
	return 0
}

// X402UseTrialCall records a free trial call, returning false when exhausted.
func X402UseTrialCall(walletAddr string) bool {
	if walletAddr == "" || x402TrialUsage[walletAddr] >= x402TrialLimit {
		return false
	}
	x402TrialUsage[walletAddr]++
	app.Log("x402", "Free trial call %d/%d for %s", x402TrialUsage[walletAddr], x402TrialLimit, walletAddr)
	return true
}

// PaymentRequirements is a single accepted way to pay, matching the x402
// "exact" scheme. maxAmountRequired is in the asset's atomic units.
type PaymentRequirements struct {
	Scheme            string            `json:"scheme"`
	Network           string            `json:"network"`
	MaxAmountRequired string            `json:"maxAmountRequired"`
	Resource          string            `json:"resource"`
	Description       string            `json:"description"`
	MimeType          string            `json:"mimeType"`
	PayTo             string            `json:"payTo"`
	MaxTimeoutSeconds int               `json:"maxTimeoutSeconds"`
	Asset             string            `json:"asset"`
	Extra             map[string]string `json:"extra,omitempty"`
}

// SettleResponse is the facilitator's settlement result.
type SettleResponse struct {
	Success     bool   `json:"success"`
	Transaction string `json:"transaction,omitempty"`
	Network     string `json:"network,omitempty"`
	Payer       string `json:"payer,omitempty"`
	ErrorReason string `json:"errorReason,omitempty"`
	Message     string `json:"message,omitempty"`
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
// one entry per accepted asset; the paying agent picks one.
func BuildPaymentRequirements(operation, resource string) []PaymentRequirements {
	cost := GetOperationCost(operation)
	if cost < 1 {
		cost = 1
	}
	var reqs []PaymentRequirements
	for _, a := range acceptedAssets() {
		reqs = append(reqs, PaymentRequirements{
			Scheme:            "exact",
			Network:           x402NetworkID,
			MaxAmountRequired: creditsToAtomic(cost, a.Decimals),
			Resource:          resource,
			Description:       "Access to " + operation,
			MimeType:          "application/json",
			PayTo:             x402PayTo,
			MaxTimeoutSeconds: 60,
			Asset:             a.Address,
			Extra:             map[string]string{"name": a.Name, "version": a.Version},
		})
	}
	return reqs
}

// WritePaymentRequired sends the standard x402 402 challenge: an HTTP 402 whose
// JSON body carries the accepted payment requirements. No custom headers — the
// body is the contract, as every x402 client expects.
func WritePaymentRequired(w http.ResponseWriter, operation, resource string) {
	reqs := BuildPaymentRequirements(operation, resource)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPaymentRequired)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"x402Version": x402Version,
		"error":       "X-PAYMENT header required",
		"accepts":     reqs,
	})
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

	body := map[string]any{"paymentPayload": payload, "paymentRequirements": req}

	// Verify.
	vres, err := facilitatorPost("/verify", body)
	if err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}
	var verify struct {
		IsValid        bool   `json:"isValid"`
		Valid          bool   `json:"valid"`
		InvalidReason  string `json:"invalidReason"`
		InvalidMessage string `json:"invalidMessage"`
	}
	_ = json.Unmarshal(vres, &verify)
	if !verify.IsValid && !verify.Valid {
		return nil, fmt.Errorf("payment invalid: %s", firstNonEmpty(verify.InvalidMessage, verify.InvalidReason, "rejected by facilitator"))
	}

	// Settle.
	sres, err := facilitatorPost("/settle", body)
	if err != nil {
		return nil, fmt.Errorf("settle: %w", err)
	}
	var settle SettleResponse
	_ = json.Unmarshal(sres, &settle)
	if !settle.Success {
		return nil, fmt.Errorf("settlement failed: %s", firstNonEmpty(settle.Message, settle.ErrorReason, "unknown"))
	}

	if h, ok := r.Context().Value(X402SettleKey).(*SettleHolder); ok && h != nil {
		h.Resp = &settle
	}
	app.Log("x402", "settled %s: tx=%s payer=%s", operation, settle.Transaction, settle.Payer)
	return &settle, nil
}

// matchRequirement picks the advertised requirement the payment was made
// against, by network (and asset when the payload names one). Falls back to the
// first requirement, which is correct for the single-asset default.
func matchRequirement(reqs []PaymentRequirements, payload map[string]any) *PaymentRequirements {
	if len(reqs) == 0 {
		return nil
	}
	net, _ := payload["network"].(string)
	net = normalizeNetwork(net)
	asset := payloadAsset(payload)
	for i := range reqs {
		if net != "" && normalizeNetwork(reqs[i].Network) != net {
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
func facilitatorPost(path string, body map[string]any) ([]byte, error) {
	endpoint := strings.TrimRight(x402FacilitatorURL, "/") + path
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if cdpConfigured() {
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		bearer, err := cdpBearer(http.MethodPost, u.Host, u.Path)
		if err != nil {
			return nil, fmt.Errorf("cdp auth: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("facilitator %s returned %d: %s", path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

// X402PriceFor returns the per-call price to invoke an operation via x402
// (e.g. "$0.05"), or "" when x402 is disabled or the operation is free. Mu
// treats 1 credit ≈ 1 US cent, so the agent price and the credit cost are the
// same number shown two ways. Used to advertise per-endpoint pricing.
func X402PriceFor(operation string) string {
	if !X402Enabled() {
		return ""
	}
	cost := GetOperationCost(operation)
	if cost < 1 {
		return ""
	}
	return fmt.Sprintf("$%d.%02d", cost/100, cost%100)
}

// X402Status returns a human-readable diagnostic of the x402 configuration
// and, when CDP credentials are present, the facilitator's advertised support —
// so an operator can certify auth on the box without exposing the secret.
func X402Status() string {
	var b strings.Builder
	fmt.Fprintf(&b, "enabled:       %v\n", X402Enabled())
	fmt.Fprintf(&b, "pay-to:        %s\n", firstNonEmpty(x402PayTo, "(X402_PAY_TO not set)"))
	fmt.Fprintf(&b, "facilitator:   %s\n", x402FacilitatorURL)
	fmt.Fprintf(&b, "network:       %s\n", x402NetworkID)
	fmt.Fprintf(&b, "version:       %d\n", x402Version)
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
	if !strings.Contains(string(data), x402NetworkID) {
		fmt.Fprintf(&b, "\nWARNING: configured network %s not in the supported list above.\n", x402NetworkID)
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

// X402SettleKey holds a *SettleHolder that VerifyAndSettle fills on success.
var X402SettleKey = x402SettleKeyType{}

// SettleHolder carries a settlement result out of VerifyAndSettle (which runs
// deep inside the tool dispatch) to the response writer.
type SettleHolder struct{ Resp *SettleResponse }

// settleWriter injects the X-PAYMENT-RESPONSE / PAYMENT-RESPONSE headers
// (base64 settlement) just before the status line is written — by which point
// settlement, if any, has completed.
type settleWriter struct {
	http.ResponseWriter
	holder *SettleHolder
	wrote  bool
}

// NewSettleWriter wraps w so a successful settlement is surfaced as the standard
// response headers.
func NewSettleWriter(w http.ResponseWriter, h *SettleHolder) http.ResponseWriter {
	return &settleWriter{ResponseWriter: w, holder: h}
}

func (s *settleWriter) WriteHeader(code int) {
	if !s.wrote {
		s.wrote = true
		if s.holder != nil && s.holder.Resp != nil {
			b, _ := json.Marshal(s.holder.Resp)
			enc := base64.StdEncoding.EncodeToString(b)
			s.Header().Set(HeaderPaymentRespV1, enc)
			s.Header().Set(HeaderPaymentRespV2, enc)
		}
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *settleWriter) Write(b []byte) (int, error) {
	if !s.wrote {
		s.WriteHeader(http.StatusOK)
	}
	return s.ResponseWriter.Write(b)
}
