package account

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/dir"
	"mu/internal/quota"
)

type stripeTransport func(*http.Request) (*http.Response, error)

func (f stripeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func stripeResponse(v any) *http.Response {
	b, _ := json.Marshal(v)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: http.Header{}}
}

func subscriptionFixture(t *testing.T) *auth.Account {
	t.Helper()
	priorQuota, _ := json.Marshal(map[string]any{"daily_credits": quota.DailyCredits(), "daily_pool_credits": quota.DailyPoolCredits(), "operations": quota.Prices()})
	testQuota, _ := json.Marshal(map[string]any{"daily_credits": 5, "daily_pool_credits": 0, "operations": quota.Prices()})
	if err := quota.Load(testQuota); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := quota.Load(priorQuota); err != nil {
			t.Error(err)
		}
	})
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_fixture")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_fixture")
	t.Setenv("SUBSCRIPTION_CENTS", "4000")
	t.Setenv("SUBSCRIPTION_CREDITS", "4000")
	oldHTTP := billingHTTP
	oldSubscriptions, oldLoad := subscriptions, subscriptionsLoadError
	subscriptions, subscriptionsLoadError = map[string]subscription{}, nil
	t.Cleanup(func() { billingHTTP = oldHTTP; subscriptions = oldSubscriptions; subscriptionsLoadError = oldLoad })
	acc := &auth.Account{ID: fmt.Sprintf("sub_%x", sha256.Sum256([]byte(t.Name())))[:20], Created: time.Now().UTC()}
	if err := auth.Create(acc); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		withLedger(func(l *ledger) bool { delete(transactions, acc.ID); delete(balances, acc.ID); return true })
	})
	return acc
}

func invoiceFixture(acc *auth.Account, id string, start time.Time) stripeInvoice {
	var out stripeInvoice
	b := fmt.Sprintf(`{"id":%q,"subscription":"sub_fixture","customer":"cus_fixture","status":"paid","currency":"usd","billing_reason":"subscription_cycle","subscription_details":{"metadata":{"plan":%q,"user_id":%q,"account_created":%q,"monthly_credits":"4000","monthly_cents":"4000"}},"lines":{"data":[{"type":"subscription","subscription":"sub_fixture","amount":4000,"period":{"start":%d,"end":%d}}]}}`, id, planMarker, acc.ID, strconv.FormatInt(acc.Created.UnixNano(), 10), start.Unix(), start.AddDate(0, 1, 0).Unix())
	if err := json.Unmarshal([]byte(b), &out); err != nil {
		panic(err)
	}
	return out
}

func TestMonthlyLedgerLifecycle(t *testing.T) {
	acc := subscriptionFixture(t)
	now := time.Now().UTC()
	invoice := invoiceFixture(acc, "in_lifecycle", now.Add(-time.Hour))
	if err := invoiceGrant(invoice); err != nil {
		t.Fatal(err)
	}
	if err := invoiceGrant(invoice); err != nil {
		t.Fatal(err)
	}
	if got := Monthly(acc.ID); got.Remaining != 4000 {
		t.Fatalf("duplicate grant: %+v", got)
	}
	if !Paid(acc.ID) {
		t.Fatal("subscriber was not recognised as paid")
	}
	if err := AddCredits(acc.ID, 100, "topup", nil); err != nil {
		t.Fatal(err)
	}
	daily := IncludedToday(acc.ID)
	if daily != 5 {
		t.Fatalf("expected a real daily allowance, got %d", daily)
	}
	settle, err := reserveIncluded(acc.ID, "test", daily+4050)
	if err != nil {
		t.Fatal(err)
	}
	if Monthly(acc.ID).Remaining != 0 || Balance(acc.ID) != 50 || quota.Available(acc.ID) != 50 {
		t.Fatal("allowance order or shared API budget incorrect")
	}
	if _, err := reserveIncluded(acc.ID, "test", 51); err == nil {
		t.Fatal("exhaustion allowed provider work")
	}
	// Reload the authoritative JSON to exercise numeric decoding and recovery.
	withLedger(func(l *ledger) bool {
		if err := data.LoadJSON("transactions.json", &transactions); err != nil {
			t.Fatal(err)
		}
		balances = map[string]*Credits{}
		rebuildFromTransactions()
		return true
	})
	recoverIncluded()
	if Monthly(acc.ID).Remaining != 4000 || Balance(acc.ID) != 100 {
		t.Fatal("restart did not refund interrupted reservation")
	}
	if err := settle(false); err != nil {
		t.Fatal(err)
	}
	if Monthly(acc.ID).Remaining != 4000 || Balance(acc.ID) != 100 {
		t.Fatal("duplicate settlement refunded twice")
	}
	settle, err = reserveIncluded(acc.ID, "test", daily+100)
	if err != nil {
		t.Fatal(err)
	}
	if err := settle(true); err != nil {
		t.Fatal(err)
	}
	if Monthly(acc.ID).Remaining != 3900 {
		t.Fatal("monthly debit missing")
	}
	// New paid month replaces the old period; old delivery cannot reset it.
	next := invoiceFixture(acc, "in_next", now)
	if err := invoiceGrant(next); err != nil {
		t.Fatal(err)
	}
	if err := invoiceGrant(invoice); err != nil {
		t.Fatal(err)
	}
	if Monthly(acc.ID).Remaining != 4000 {
		t.Fatal("renewal or out-of-order grant failed")
	}
	withLedger(func(l *ledger) bool {
		if monthly(l, acc.ID, now.AddDate(0, 2, 0)).Remaining != 0 {
			t.Fatal("unused allowance rolled over")
		}
		return true
	})
	if Balance(acc.ID) != 100 {
		t.Fatal("expiry changed prepaid balance")
	}
	if err := TransferCredits(acc.ID, "monthly-transfer-recipient", 101); err == nil {
		t.Fatal("monthly credit was transferable")
	}
}

func TestMonthlyConcurrentReservations(t *testing.T) {
	acc := subscriptionFixture(t)
	now := time.Now().UTC()
	if err := grantMonthly(acc.ID, "sub_concurrent", "in_concurrent", 100, now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	daily := IncludedToday(acc.ID)
	if err := chargeIncluded(acc.ID, daily, "daily", nil); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var countMu sync.Mutex
	successes := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			settle, err := reserveIncluded(acc.ID, "test", 10)
			if err == nil {
				if err := settle(true); err != nil {
					t.Error(err)
				}
				countMu.Lock()
				successes++
				countMu.Unlock()
			}
		}()
	}
	wg.Wait()
	if successes != 10 || Monthly(acc.ID).Remaining != 0 {
		t.Fatalf("overspend: %d", successes)
	}
}

func TestSubscriptionInvoicesAndReorderedEvents(t *testing.T) {
	acc := subscriptionFixture(t)
	now := time.Now().UTC()
	i := invoiceFixture(acc, "in_events", now.Add(-time.Hour))
	remote := stripeSubscription{ID: i.Subscription, Customer: i.Customer, Status: "active", PeriodEnd: now.AddDate(0, 1, 0).Unix(), LatestInvoice: i.ID, Metadata: i.SubscriptionDetails.Metadata}
	billingHTTP = &http.Client{Transport: stripeTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Stripe-Version") != stripeVersion {
			t.Error("unversioned response")
		}
		switch r.URL.Path {
		case "/v1/invoices/" + i.ID:
			return stripeResponse(i), nil
		case "/v1/subscriptions/sub_fixture":
			return stripeResponse(remote), nil
		}
		return nil, errors.New("unexpected Stripe path")
	})}
	// An invoice can arrive before checkout completion.
	raw, _ := json.Marshal(map[string]any{"id": i.ID})
	if err := subscriptionEvent(context.Background(), "invoice.paid", raw); err != nil {
		t.Fatal(err)
	}
	if Monthly(acc.ID).Remaining != 4000 {
		t.Fatal("paid invoice did not grant")
	}
	remote.CancelAtEnd = true
	stale, _ := json.Marshal(map[string]any{"id": remote.ID, "metadata": remote.Metadata, "cancel_at_period_end": false})
	if err := subscriptionEvent(context.Background(), "customer.subscription.updated", stale); err != nil {
		t.Fatal(err)
	}
	if !subscriptions[acc.ID].CancelAtEnd {
		t.Fatal("stale event overwrote canonical cancellation")
	}
	remote.Status = "past_due"
	i.Status = "open"
	if err := subscriptionEvent(context.Background(), "invoice.payment_failed", raw); err != nil {
		t.Fatal(err)
	}
	if Monthly(acc.ID).Remaining != 4000 {
		t.Fatal("failed payment revoked an already paid period")
	}
	unpaid := invoiceFixture(acc, "in_unpaid", now)
	unpaid.Status = "open"
	if err := invoiceGrant(unpaid); err != nil {
		t.Fatal(err)
	}
	if Monthly(acc.ID).Invoice != "in_events" {
		t.Fatal("unpaid renewal granted usage")
	}
	wrong := invoiceFixture(acc, "in_wrong", now)
	wrong.Lines.Data[0].Amount = 1
	if err := invoiceGrant(wrong); err == nil {
		t.Fatal("mismatched amount granted")
	}
	wrong = invoiceFixture(acc, "in_reused_name", now)
	wrong.SubscriptionDetails.Metadata["account_created"] = "old-account"
	if err := invoiceGrant(wrong); err != nil {
		t.Fatal(err)
	}
	if Monthly(acc.ID).Invoice != "in_events" {
		t.Fatal("historic invoice credited a reused account name")
	}
}

func TestCheckoutRetryAndOwnership(t *testing.T) {
	acc := subscriptionFixture(t)
	posts := 0
	var firstKey, firstBody string
	remoteSession := subscriptionCheckout{ID: "cs_fixture", URL: "https://checkout.stripe.com/c/test", Status: "open", Mode: "subscription", Metadata: map[string]string{"user_id": acc.ID, "plan": planMarker}}
	billingHTTP = &http.Client{Transport: stripeTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v1/webhook_endpoints" {
			return stripeResponse(map[string]any{"data": []any{map[string]any{"id": "we_test", "url": "https://micro.test/stripe/webhook", "status": "enabled", "enabled_events": []string{"*"}}}}), nil
		}
		if r.Method == "GET" {
			return stripeResponse(remoteSession), nil
		}
		posts++
		b, _ := io.ReadAll(r.Body)
		if posts == 1 {
			firstKey, firstBody = r.Header.Get("Idempotency-Key"), string(b)
			return nil, errors.New("lost response")
		}
		if firstKey == "" || r.Header.Get("Idempotency-Key") != firstKey || string(b) != firstBody {
			t.Error("retry changed checkout identity or terms")
		}
		v, _ := url.ParseQuery(string(b))
		if v.Get("mode") != "subscription" || v.Get("line_items[0][price_data][recurring][interval]") != "month" || v.Get("subscription_data[metadata][monthly_credits]") != "4000" {
			t.Error("incorrect subscription form")
		}
		return stripeResponse(remoteSession), nil
	})}
	if _, err := startSubscription(context.Background(), acc, "https://micro.test"); err == nil {
		t.Fatal("expected network error")
	}
	// Simulated restart retains the attempt, not a new idempotency key.
	subscriptions = nil
	if err := data.LoadJSON("subscriptions.json", &subscriptions); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUBSCRIPTION_CENTS", "5000")
	if _, err := startSubscription(context.Background(), acc, "https://micro.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := startSubscription(context.Background(), acc, "https://micro.test"); err != nil {
		t.Fatal(err)
	}
	if posts != 2 {
		t.Fatal("double click created another checkout")
	}
	session, err := auth.CreateSession(acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/account/subscription", strings.NewReader("action=cancel"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
	w := httptest.NewRecorder()
	SubscriptionHandler(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal("mutation accepted without CSRF")
	}
	remoteSession.Metadata["user_id"] = "somebody-else"
	r = httptest.NewRequest("GET", "/account/subscription?session_id=cs_fixture", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
	w = httptest.NewRecorder()
	SubscriptionHandler(w, r)
	if w.Code != http.StatusServiceUnavailable || Monthly(acc.ID).Remaining != 0 {
		t.Fatal("foreign checkout accepted")
	}
}

func TestStripeRetriesLedgerFailures(t *testing.T) {
	acc := subscriptionFixture(t)
	// Force atomic rename failure without changing the process home or touching
	// any real data: test binaries already use a disposable store.
	file := filepath.Join(dir.Data(), "transactions.json")
	if err := data.SaveJSON("transactions.json", transactions); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(file, file+".backup"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	var restored sync.Once
	restore := func() { restored.Do(func() { os.Remove(file); os.Rename(file+".backup", file) }) }
	t.Cleanup(restore)
	payload := fmt.Sprintf(`{"type":"checkout.session.completed","data":{"object":{"id":"cs_failure","mode":"payment","payment_status":"paid","amount_total":500,"metadata":{"user_id":%q,"credits":"500"}}}}`, acc.ID)
	send := func() int {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		r := httptest.NewRequest("POST", "/stripe/webhook", strings.NewReader(payload))
		r.Header.Set("Stripe-Signature", "t="+ts+",v1="+computeHMAC(ts+"."+payload, "whsec_fixture"))
		w := httptest.NewRecorder()
		HandleStripeWebhook(w, r)
		return w.Code
	}
	if send() != 500 || Balance(acc.ID) != 0 {
		t.Fatal("failed ledger write acknowledged or mutated balance")
	}
	i := invoiceFixture(acc, "in_failure", time.Now().Add(-time.Hour))
	if err := invoiceGrant(i); err == nil || Monthly(acc.ID).Remaining != 0 {
		t.Fatal("failed monthly grant was retained")
	}
	restore()
	if send() != 200 || Balance(acc.ID) != 500 {
		t.Fatal("retry did not credit")
	}
	if send() != 200 || Balance(acc.ID) != 500 {
		t.Fatal("duplicate webhook credited twice")
	}
}

func TestSubscriptionCancellationAndDeletion(t *testing.T) {
	acc := subscriptionFixture(t)
	now := time.Now().UTC()
	i := invoiceFixture(acc, "in_cancel", now.Add(-time.Hour))
	if err := invoiceGrant(i); err != nil {
		t.Fatal(err)
	}
	remote := stripeSubscription{ID: "sub_fixture", Customer: "cus_fixture", Status: "active", PeriodEnd: now.AddDate(0, 1, 0).Unix(), LatestInvoice: i.ID, Metadata: i.SubscriptionDetails.Metadata}
	subscriptions[acc.ID] = subscription{ID: remote.ID, AccountCreated: strconv.FormatInt(acc.Created.UnixNano(), 10), Status: "active", Cents: 4000, Credits: 4000}
	mutations := 0
	failDelete := true
	billingHTTP = &http.Client{Transport: stripeTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasPrefix(r.URL.Path, "/v1/invoices/") {
			return stripeResponse(i), nil
		}
		if r.Method == "POST" {
			mutations++
			r.ParseForm()
			if r.Form.Get("cancel_at_period_end") != "true" {
				t.Error("cancellation removes paid period")
			}
			remote.CancelAtEnd = true
		}
		if r.Method == "DELETE" {
			if failDelete {
				return nil, errors.New("Stripe unavailable")
			}
			remote.Status = "canceled"
		}
		return stripeResponse(remote), nil
	})}
	session, err := auth.CreateSession(acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/account/subscription", strings.NewReader("action=cancel"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
	r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
	w := httptest.NewRecorder()
	SubscriptionHandler(w, r)
	if w.Code != 303 || mutations != 1 || !subscriptions[acc.ID].CancelAtEnd || Monthly(acc.ID).Remaining != 4000 {
		t.Fatal("cancellation did not preserve paid allowance")
	}
	if err := EndSubscription(context.Background(), acc.ID); err == nil {
		t.Fatal("deletion allowed with uncertain ongoing billing")
	}
	failDelete = false
	if err := EndSubscription(context.Background(), acc.ID); err != nil {
		t.Fatal(err)
	}
	if subscriptions[acc.ID].ID != "" || remote.Status != "canceled" {
		t.Fatal("billing survived account deletion")
	}
}
