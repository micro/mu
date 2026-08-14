package x402

// Nobody is charged for an answer they did not get.
//
// The two halves of a payment are separate calls in the protocol, and this
// instance was collapsing them: verify and settle both happened at the door,
// before the tool ran and therefore before anything had read the arguments. So
// a request that failed afterwards was a charge with no answer.
//
// That is not a corner. It was found by typing a plausible command —
// `mu x402 call weather_forecast lat=51.5` — where lat arrived as a string,
// the tool refused it, and 0.01 USDC went anyway. Anything that fails after the
// gate does this: a bad argument, an upstream provider down, a panic.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSucceededIsWhatIsWorthCharging(t *testing.T) {
	for _, c := range []struct {
		what string
		code int
		body string
		want bool
	}{
		{"an ordinary answer", 200, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"text":"hi"}]}}`, true},

		// The case that cost money. MCP answers a failed tool call with 200 and
		// a JSON-RPC error, so the status line alone says success.
		{"a JSON-RPC error", 200, `{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"bad arguments"}}`, false},
		{"a tool that set isError", 200, `{"jsonrpc":"2.0","id":1,"result":{"isError":true,"content":[]}}`, false},
		{"an explicit null error", 200, `{"jsonrpc":"2.0","id":1,"error":null,"result":{}}`, true},

		{"a server error", 500, `{"jsonrpc":"2.0","id":1,"result":{}}`, false},
		{"a refusal", 402, `{"error":"pay up"}`, false},
		{"a redirect", 302, ``, false},

		// Not JSON-RPC at all. A 2xx is success by the only measure there is,
		// and refusing to charge for every non-JSON answer would be a worse
		// error than the one being fixed.
		{"plain text", 200, `hello`, true},
		{"empty", 200, ``, true},
	} {
		if got := succeeded(c.code, []byte(c.body)); got != c.want {
			t.Errorf("%s: succeeded(%d, %q) = %v, want %v", c.what, c.code, c.body, got, c.want)
		}
	}
}

// settleWriter holds everything back until Finish, and Finish is what decides.
func TestTheResponseIsHeldUntilTheMoneyQuestionIsAsked(t *testing.T) {
	rec := httptest.NewRecorder()
	w := NewSettleWriter(rec, &SettleHolder{})

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`)) //nolint:errcheck

	if rec.Body.Len() != 0 {
		t.Error("the body reached the client before anything decided whether to charge")
	}

	Finish(w)

	if rec.Code != http.StatusOK {
		t.Errorf("status %d after Finish, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"jsonrpc"`) {
		t.Errorf("the body never arrived: %q", rec.Body.String())
	}
}

// A failed call settles nothing, and says nothing about a payment either — a
// receipt header on a failure would tell the caller they had been charged.
func TestAFailedCallCarriesNoReceipt(t *testing.T) {
	rec := httptest.NewRecorder()
	// A holder with a Verified whose Settle would panic if it were reached:
	// there is no facilitator here, so reaching it is the failure.
	holder := &SettleHolder{Verified: &Verified{}}
	w := NewSettleWriter(rec, holder)

	w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"bad arguments"}}`)) //nolint:errcheck
	Finish(w)

	if holder.Resp != nil {
		t.Error("a failed call settled a payment")
	}
	for _, h := range []string{HeaderPaymentRespV1, HeaderPaymentRespV2} {
		if rec.Header().Get(h) != "" {
			t.Errorf("a failed call carried %s, which tells the caller they were charged", h)
		}
	}
	// And the error still reaches them.
	if !strings.Contains(rec.Body.String(), "bad arguments") {
		t.Errorf("the error was swallowed: %q", rec.Body.String())
	}
}

// The payer is known while the tool runs, not only once the money has moved.
//
// PayerFrom used to read the settlement. Deferring settlement to the end would
// have made it empty for the whole of the request that needs it — an
// account-scoped tool reached by paying asks who is calling, and a blank answer
// is a refusal of a call that has been paid for.
func TestThePayerIsKnownBeforeTheMoneyMoves(t *testing.T) {
	const from = "0x4160A86303eeBA12fc0A3FFB8480A9d2D1eAb7A1"

	v := &Verified{payload: map[string]any{
		"payload": map[string]any{
			"authorization": map[string]any{"from": from},
		},
	}}

	if got := v.Payer(); got != strings.ToLower(from) {
		t.Fatalf("Verified.Payer() = %q, want the lowercased signer", got)
	}

	ctx := context.WithValue(context.Background(), X402SettleKey, &SettleHolder{Verified: v})
	if got := PayerFrom(ctx); got != strings.ToLower(from) {
		t.Errorf("PayerFrom during the call = %q — the tool cannot tell who is paying", got)
	}
}

// And a request with no payment at all has no payer, rather than an empty
// string that some caller might mistake for one.
func TestNoPaymentMeansNoPayer(t *testing.T) {
	if got := PayerFrom(context.Background()); got != "" {
		t.Errorf("PayerFrom on an unpaid request = %q", got)
	}
	ctx := context.WithValue(context.Background(), X402SettleKey, &SettleHolder{})
	if got := PayerFrom(ctx); got != "" {
		t.Errorf("PayerFrom with nothing verified = %q", got)
	}
}

// A successful settlement is reported in both headers, whichever version the
// client speaks.
func TestASettlementIsReportedInBothHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	holder := &SettleHolder{Resp: &SettleResponse{Success: true, Transaction: "0xabc", Payer: "0xdef"}}
	w := NewSettleWriter(rec, holder)
	// No Verified, so finish() will not try to settle; this is about the
	// reporting shape once something has been settled.
	sw := w.(*settleWriter)
	sw.code = http.StatusOK
	sw.body.WriteString(`{"result":{}}`)
	b, _ := json.Marshal(holder.Resp)
	enc := base64.StdEncoding.EncodeToString(b)
	sw.Header().Set(HeaderPaymentRespV1, enc)
	sw.Header().Set(HeaderPaymentRespV2, enc)
	sw.finish()

	for _, h := range []string{HeaderPaymentRespV1, HeaderPaymentRespV2} {
		raw := rec.Header().Get(h)
		if raw == "" {
			t.Fatalf("%s is missing", h)
		}
		dec, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			t.Fatalf("%s is not base64: %v", h, err)
		}
		var got SettleResponse
		if err := json.Unmarshal(dec, &got); err != nil {
			t.Fatalf("%s is not a settlement: %v", h, err)
		}
		if got.Transaction != "0xabc" {
			t.Errorf("%s carries tx %q", h, got.Transaction)
		}
	}
}
