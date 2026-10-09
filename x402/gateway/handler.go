package gateway

import (
	"bytes"
	"context"
	"io"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
	"mu/internal/usage"
	"mu/service/wallet"
	x402 "mu/x402/payment"
	"net/http"
	"strings"
)

// Handler owns credential normalization, wallet authentication and x402 settlement for both API doors.
func Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = api.CredentialRequest(r)
		// MCP authorization: an unauthenticated call to a tool that needs an
		// account gets a 401 naming the resource metadata, which is how a
		// client discovers it should start an OAuth flow. The discovery
		// documents existed without this and were never fetched, so the
		// standard way of connecting quietly did not work.
		//
		// Only auth-requiring tools challenge. A blanket 401 would make news
		// and weather unreachable without an account.
		// A wallet that signed instead of paying. Verified once, here,
		// because the nonce may only be spent once — checking it again
		// deeper in would refuse the caller's own second look.
		// Two doors dispatch tools — /mcp for something choosing one, and
		// /api/v1/ for something that already knows which it wants — and
		// everything below has to happen for both. It used to say /mcp four
		// times, which is how a second door starts out unauthenticated and
		// unpriced: the handler is the easy half, and this is the half
		// nobody remembers exists.
		//
		// Read the body once. It was read twice, restored twice, and parsed
		// twice for two questions about the same tool.
		if api.ToolDispatch(r.URL.Path) {
			host := strings.TrimPrefix(strings.TrimPrefix(app.BaseURL(r), "https://"), "http://")
			r, _ = wallet.AuthenticateRequest(r, strings.TrimRight(host, "/"))

			var body []byte
			if r.Method == http.MethodPost {
				var err error
				body, err = io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
				r.Body.Close()
				if err != nil {
					app.RespondError(w, http.StatusBadRequest, "Could not read request")
					return
				}
				if len(body) > 1<<20 {
					app.RespondError(w, http.StatusRequestEntityTooLarge, "Request too large")
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
			}
			tool := api.RequestTool(r.URL.Path, body)

			if api.ToolNeedsAuth(tool) {
				if _, err := auth.GetSession(r); err != nil && !x402.HasPayment(r) &&
					wallet.SignerFrom(r.Context()) == "" {
					origin := app.BaseURL(r)
					w.Header().Set("WWW-Authenticate",
						`Bearer resource_metadata="`+origin+`/.well-known/oauth-protected-resource"`)
					app.RespondError(w, http.StatusUnauthorized, "authentication required")
					return
				}
			}

			// x402: gate metered tool calls. Both doors are public, so the
			// payment handshake lives here where auth + wallet are in
			// scope. A metered call with no session gets the standard 402
			// challenge; one bearing a payment header is routed to the
			// facilitator for verify+settle by the tool's QuotaCheck.
			//
			// Metered, not merely priced. A tool having a wallet operation
			// is not the same as it costing anything: news, web fetch,
			// quran and video search are zero on purpose. Gating on "has an
			// operation" charged an anonymous caller for all four, so the
			// free tier was unreachable and an agent that found this
			// endpoint mid-task met a demand for USDC on its first call.
			if op := api.ToolWalletOp(tool); x402.Enabled() && op != "" && quota.Metered(op) {
				// The public origin, not r.Host: behind the proxy r.Host is
				// the loopback port, and an x402 client checks this field
				// against what it is calling.
				resource := app.BaseURL(r) + r.URL.Path
				if x402.HasPayment(r) {
					holder := &x402.SettleHolder{}
					ctx := context.WithValue(r.Context(), x402.X402ContextKey, true)
					ctx = context.WithValue(ctx, x402.X402SettleKey, holder)
					r = r.WithContext(ctx)
					w = x402.NewSettleWriter(w, holder)
					// Nothing is written or charged until this runs: the
					// payment settles only if the response says the work
					// succeeded, and the response is held back until then
					// because the receipt is a header and the verdict is in
					// the body.
					defer x402.Finish(w)
				} else if who, blocked, reason := payer(r, op); blocked {
					// No listing. A discovery extension used to ride along
					// in this challenge, describing the refused tool so a
					// facilitator could index it, behind a setting that was
					// off by default and never turned on. See
					// x402/payment/bazaar.go for why the whole idea went.
					if x402.WritePaymentRequired(w, op, resource, nil, reason) {
						// Count the refusal. Calls are recorded inside the
						// dispatcher, which this returns before reaching,
						// so every call turned away at the door was absent
						// from the usage figures — including the free ones
						// this gate should never have been refusing. The
						// number that would have shown the mistake could
						// not see it.
						usage.Record("mcp-refused", op, who)
						usage.RecordActivity(usage.Activity{Surface: "mcp", Operation: op, Account: who, Status: 402, Outcome: "credits or quota"})
						return
					}
					// Nothing to charge: let it through rather than
					// inventing a price. Belt and braces with Metered
					// above, so neither check alone can paywall a free
					// tool again.
				}
			}
		}

		next.ServeHTTP(w, r)
	})
}
