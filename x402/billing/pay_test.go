package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/pay"
	pb "mu/proto/assistant/v1"
)

func TestMuPayCheckoutWebhookAndLedgerRecovery(t *testing.T) {
	acc := subscriptionFixture(t)
	old := pay.Backend
	pay.Backend = Pay{}
	t.Cleanup(func() { pay.Backend = old })
	t.Setenv("MU_PAY_GBP_CREDITS_PER_POUND", "100")
	requests := 0
	billingHTTP = &http.Client{Transport: stripeTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.Method != "POST" || r.URL.Path != "/v1/checkout/sessions" {
			t.Fatal("unexpected Stripe request", r.URL)
		}
		if e := r.ParseForm(); e != nil {
			t.Fatal(e)
		}
		f := r.Form
		if f.Get("line_items[0][quantity]") != "3" || f.Get("line_items[0][price_data][unit_amount]") != "300" || f.Get("line_items[0][price_data][currency]") != "gbp" {
			t.Fatal("wrong purchase", f)
		}
		if f.Get("metadata[owner]") != acc.ID || f.Get("metadata[user_id]") != "" || f.Get("customer") != "" || f.Get("customer_email") != "" {
			t.Fatal("guest associated with owner billing profile", f)
		}
		id := f.Get("metadata[mu_pay]")
		if r.Header.Get("Idempotency-Key") != "mu-pay-"+id {
			t.Fatal("no stable retry key")
		}
		return stripeResponse(map[string]any{"id": "cs_" + id, "url": "https://checkout.stripe.com/c/pay/test", "expires_at": time.Now().Add(24 * time.Hour).Unix()}), nil
	})}
	policy := &pay.Policy{Enabled: true, Price: 300, Currency: "gbp", Minutes: 60, ReturnUrl: "https://malten.me/session"}
	q := &pb.CheckoutRequest{Reference: "billing-recovery", Participants: 3}
	s, e := pay.Checkout(context.Background(), acc.ID, "malten", "https://micro.mu", policy, "secret", q)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = pay.Checkout(context.Background(), acc.ID, "malten", "https://micro.mu", policy, "secret", q); e != nil || requests != 1 {
		t.Fatal("duplicate checkout", e)
	}
	object := map[string]any{"id": "cs_" + s.Id, "mode": "payment", "payment_status": "paid", "amount_total": 900, "currency": "gbp", "customer": "cus_guest", "metadata": map[string]string{"mu_pay": s.Id, "owner": acc.ID, "agent": "malten"}}
	send := func(valid bool) int {
		raw, _ := json.Marshal(map[string]any{"type": "checkout.session.completed", "data": map[string]any{"object": object}})
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		sig := computeHMAC(ts+"."+string(raw), "whsec_fixture")
		if !valid {
			sig = "invalid"
		}
		r := httptest.NewRequest("POST", "/stripe/webhook", strings.NewReader(string(raw)))
		r.Header.Set("Stripe-Signature", "t="+ts+",v1="+sig)
		w := httptest.NewRecorder()
		HandleStripeWebhook(w, r)
		return w.Code
	}
	before := Balance(acc.ID)
	if Paid(acc.ID) {
		t.Fatal("fixture is already personally funded")
	}
	if send(false) != 400 || Balance(acc.ID) != before {
		t.Fatal("unsigned payment accepted")
	}
	object["amount_total"] = 899
	if send(true) != 500 || Balance(acc.ID) != before {
		t.Fatal("mismatched payment accepted")
	}
	object["amount_total"] = 900
	// Simulate a process dying after the credit transaction commits and before
	// the paid-session receipt is saved. Rehydrate the ledger from disk.
	var records map[string]pay.Record
	if e = data.LoadJSON("pay/sessions.json", &records); e != nil {
		t.Fatal(e)
	}
	record := records[s.Id]
	if e = (Pay{}).Credit(context.Background(), record); e != nil {
		t.Fatal(e)
	}
	withLedger(func(l *ledger) bool {
		if e := data.LoadJSON("transactions.json", &transactions); e != nil {
			t.Fatal(e)
		}
		if e := data.LoadJSON("wallets.json", &balances); e != nil {
			t.Fatal(e)
		}
		return true
	})
	for i := 0; i < 3; i++ {
		if send(true) != 200 {
			t.Fatal("webhook retry failed")
		}
	}
	if Balance(acc.ID) != before+900 {
		t.Fatalf("payment credited more than once: %d", Balance(acc.ID))
	}
	if Paid(acc.ID) {
		t.Fatal("agent revenue granted personally-funded privileges")
	}
	for _, tx := range Transactions(acc.ID, 20) {
		if tx.Metadata["source"] == "mu_pay" && TransactionLabel(tx) != "Agent revenue" {
			t.Fatal("guest payment shown as personal top-up")
		}
	}
	restored, e := pay.Read(acc.ID, "malten", s.Id)
	if e != nil || restored.Status != "paid" || restored.EndsAt != 0 {
		t.Fatal("receipt recovery", restored, e)
	}
	owner, e := auth.GetAccount(acc.ID)
	if e != nil || owner.Customer != "" {
		t.Fatal("guest customer saved on owner", owner, e)
	}
	active, e := pay.Start(acc.ID, "malten", s.Id)
	if e != nil || active.EndsAt-active.StartedAt != 3600 {
		t.Fatal(fmt.Sprint(active, e))
	}
}

func TestMuPayLateCheckoutRetry(t *testing.T) {
	for _, created := range []bool{false, true} {
		t.Run(fmt.Sprintf("created_before_timeout_%t", created), func(t *testing.T) {
			acc := subscriptionFixture(t)
			old := pay.Backend
			pay.Backend = Pay{}
			t.Cleanup(func() { pay.Backend = old })
			attempts, creations := 0, 0
			firstForm, firstKey := "", ""
			expires := time.Now().Add(24 * time.Hour).Unix()
			billingHTTP = &http.Client{Transport: stripeTransport(func(r *http.Request) (*http.Response, error) {
				attempts++
				if e := r.ParseForm(); e != nil {
					t.Fatal(e)
				}
				if r.Form.Get("expires_at") != "" {
					t.Fatal("retry carries a stale absolute expiry")
				}
				key := r.Header.Get("Idempotency-Key")
				form := r.Form.Encode()
				if attempts == 1 {
					firstKey, firstForm = key, form
					if created {
						creations++
					}
					return nil, fmt.Errorf("lost connection")
				}
				if firstKey != key || firstForm != form {
					t.Fatal("retry changed Stripe idempotency parameters")
				}
				if !created {
					creations++
				}
				return stripeResponse(map[string]any{"id": "cs_" + r.Form.Get("metadata[mu_pay]"), "url": "https://checkout.stripe.com/c/pay/test", "expires_at": expires}), nil
			})}
			policy := &pay.Policy{Enabled: true, Price: 300, Currency: "gbp", Minutes: 60, ReturnUrl: "https://malten.me/session"}
			q := &pb.CheckoutRequest{Reference: "late-retry", Participants: 3}
			if _, e := pay.Checkout(context.Background(), acc.ID, "malten", "https://micro.mu", policy, "", q); e == nil {
				t.Fatal("expected timeout")
			}
			var records map[string]pay.Record
			if e := data.LoadJSON("pay/sessions.json", &records); e != nil {
				t.Fatal(e)
			}
			id := ""
			for k, r := range records {
				if r.Owner == acc.ID {
					r.Session.CreatedAt = time.Now().Add(-40 * time.Minute).Unix()
					r.Expires = time.Now().Add(20 * time.Minute).Unix()
					records[k] = r
					id = k
				}
			}
			if id == "" {
				t.Fatal("pending checkout not saved")
			}
			if e := data.SaveJSON("pay/sessions.json", records); e != nil {
				t.Fatal(e)
			}
			s, e := pay.Checkout(context.Background(), acc.ID, "malten", "https://micro.mu", policy, "", q)
			if e != nil || s.Id != id {
				t.Fatal("late retry failed", s, e)
			}
			if e = data.LoadJSON("pay/sessions.json", &records); e != nil {
				t.Fatal(e)
			}
			if records[id].Expires != expires {
				t.Fatal("Stripe expiry not retained")
			}
			if _, e = pay.Checkout(context.Background(), acc.ID, "malten", "https://micro.mu", policy, "", q); e != nil || attempts != 2 || creations != 1 {
				t.Fatal("duplicate purchase", attempts, creations, e)
			}
		})
	}
}
