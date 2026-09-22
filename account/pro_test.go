package account

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
)

func TestProDefaultsAndSignupDestination(t *testing.T) {
	acc := subscriptionFixture(t)
	t.Setenv("SUBSCRIPTION_CENTS", "")
	t.Setenv("SUBSCRIPTION_CREDITS", "")
	t.Setenv("INVITE_ONLY", "false")
	t.Setenv("GOOGLE_CLIENT_ID", "test")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test")
	if p, ok := MonthlyPlan(); !ok || p.Cents != 4000 || p.Credits != 4000 {
		t.Fatalf("plan=%+v enabled=%v", p, ok)
	}
	t.Setenv("SUBSCRIPTION_CENTS", "5000")
	if _, ok := MonthlyPlan(); ok {
		t.Fatal("partial price override was sold with a default allowance")
	}
	t.Setenv("SUBSCRIPTION_CENTS", "")
	t.Setenv("SUBSCRIPTION_CREDITS", "10000")
	if _, ok := MonthlyPlan(); ok {
		t.Fatal("partial allowance override was sold at the default price")
	}
	t.Setenv("SUBSCRIPTION_CREDITS", "")
	want := "/account?plan=pro#subscription"
	r := httptest.NewRequest("GET", "/pricing", nil)
	page := MonthlyPricingHTML(r)
	if !strings.Contains(page, `href="/signup?redirect=`+url.QueryEscape(want)+`"`) || !strings.Contains(page, "$40/month") {
		t.Fatal("Pro purchase entry missing")
	}
	r = httptest.NewRequest("GET", "/signup?redirect="+url.QueryEscape(want), nil)
	w := httptest.NewRecorder()
	Signup(w, r)
	for _, entry := range []string{"Pro · $40/month", `action="/signup?redirect=`, `href="/login?redirect=`, `href="/oauth2/google?redirect=`} {
		if !strings.Contains(w.Body.String(), entry) {
			t.Fatalf("signup lost %s", entry)
		}
	}
	session, err := auth.CreateSession(acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest("GET", "/account?plan=pro", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
	if page := subscriptionSummary(r, acc); !strings.Contains(page, ">Plan<") || !strings.Contains(page, "Continue to payment") {
		t.Fatal("selected plan missing in Account")
	}
	t.Setenv("SUBSCRIPTION_CENTS", "0")
	if _, ok := MonthlyPlan(); ok {
		t.Fatal("operator cannot disable new subscriptions")
	}
	t.Setenv("SUBSCRIPTION_CENTS", "4000")
	t.Setenv("SUBSCRIPTION_CREDITS", "4000")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "")
	if _, ok := MonthlyPlan(); ok {
		t.Fatal("plan enabled without signed webhooks")
	}
}

func TestProSignupCheckoutAndReturn(t *testing.T) {
	_ = subscriptionFixture(t)
	t.Setenv("ADMIN", "operator")
	t.Setenv("INVITE_ONLY", "false")
	captcha := app.NewCaptchaChallenge()
	var a, b int
	fmt.Sscanf(captcha.Question, "What is %d + %d?", &a, &b)
	form := url.Values{"id": {"pro_signup_flow"}, "secret": {"test-password"}, "captcha": {strconv.Itoa(a + b)}, "captcha_nonce": {captcha.Nonce}, "captcha_ts": {captcha.Timestamp}, "captcha_sig": {captcha.Signature}}
	destination := "/account?plan=pro#subscription"
	r := httptest.NewRequest("POST", "/signup?redirect="+url.QueryEscape(destination), strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	Signup(w, r)
	if w.Code != 302 || w.Header().Get("Location") != destination {
		t.Fatalf("signup: %d %s", w.Code, w.Body.String())
	}
	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "session" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("signup lost session")
	}
	acc, err := auth.GetAccount("pro_signup_flow")
	if err != nil {
		t.Fatal(err)
	}
	var remote stripeSubscription
	invoice := invoiceFixture(acc, "in_signup", time.Now().Add(-time.Minute))
	billingHTTP = &http.Client{Transport: stripeTransport(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/v1/webhook_endpoints":
			return stripeResponse(map[string]any{"data": []any{map[string]any{"id": "we_flow", "url": "https://micro.test/stripe/webhook", "status": "enabled", "enabled_events": []string{"*"}}}}), nil
		case "/v1/checkout/sessions":
			r.ParseForm()
			if r.PostForm.Get("line_items[0][price_data][product_data][name]") != "Micro Pro" || r.PostForm.Get("line_items[0][price_data][unit_amount]") != "4000" || r.PostForm.Get("mode") != "subscription" {
				t.Fatal("wrong recurring plan")
			}
			meta := map[string]string{}
			for _, key := range []string{"plan", "user_id", "account_created", "checkout_attempt", "monthly_credits", "monthly_cents"} {
				meta[key] = r.PostForm.Get("subscription_data[metadata][" + key + "]")
			}
			remote = stripeSubscription{ID: "sub_fixture", Customer: "cus_fixture", Status: "active", PeriodEnd: time.Now().AddDate(0, 1, 0).Unix(), LatestInvoice: invoice.ID, Metadata: meta}
			return stripeResponse(subscriptionCheckout{ID: "cs_signup", URL: "https://checkout.stripe.com/c/test"}), nil
		case "/v1/checkout/sessions/cs_signup":
			return stripeResponse(subscriptionCheckout{ID: "cs_signup", Mode: "subscription", Status: "complete", Subscription: remote.ID, Metadata: map[string]string{"plan": planMarker, "user_id": acc.ID}}), nil
		case "/v1/subscriptions/sub_fixture":
			return stripeResponse(remote), nil
		case "/v1/invoices/in_signup":
			return stripeResponse(invoice), nil
		default:
			t.Fatalf("unexpected Stripe request %s", r.URL.Path)
			return nil, nil
		}
	})}
	r = httptest.NewRequest("POST", "https://micro.test/account/subscription", strings.NewReader("action=subscribe"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
	w = httptest.NewRecorder()
	SubscriptionHandler(w, r)
	if w.Code != 303 || w.Header().Get("Location") != "https://checkout.stripe.com/c/test" {
		t.Fatalf("checkout: %d %s", w.Code, w.Body.String())
	}
	for range 2 {
		r = httptest.NewRequest("GET", "https://micro.test/account/subscription?session_id=cs_signup", nil)
		r.AddCookie(cookie)
		w = httptest.NewRecorder()
		SubscriptionHandler(w, r)
		if w.Code != 303 || Monthly(acc.ID).Remaining != 4000 || !Paid(acc.ID) {
			t.Fatalf("paid return failed: %d %s", w.Code, w.Body.String())
		}
	}
	r = httptest.NewRequest("GET", "/account", nil)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	Account(w, r)
	for _, text := range []string{">Plan<", ">Pro<", "$40/month", "4,000 / 4,000", "Cancel renewal"} {
		if !strings.Contains(w.Body.String(), text) {
			t.Fatalf("Account lost %s", text)
		}
	}
}

func TestProWebhookSetupPreservesOtherEventsAndSites(t *testing.T) {
	_ = subscriptionFixture(t)
	existing := []string{"checkout.session.completed", "charge.refunded"}
	updates := 0
	billingHTTP = &http.Client{Transport: stripeTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			return stripeResponse(map[string]any{"data": []any{
				map[string]any{"id": "we_other", "url": "https://other.test/stripe/webhook", "status": "enabled", "enabled_events": []string{"charge.succeeded"}},
				map[string]any{"id": "we_micro", "url": "https://micro.test/stripe/webhook", "status": "enabled", "enabled_events": existing},
			}}), nil
		}
		if r.URL.Path != "/v1/webhook_endpoints/we_micro" {
			t.Fatalf("changed another site's webhook: %s", r.URL.Path)
		}
		r.ParseForm()
		existing = r.PostForm["enabled_events[]"]
		if len(r.PostForm) != 1 || !slices.Contains(existing, "charge.refunded") {
			t.Fatal("replaced webhook setup")
		}
		updates++
		return stripeResponse(map[string]any{"enabled_events": existing}), nil
	})}
	for range 2 {
		if err := ensureSubscriptionWebhook(context.Background(), "https://micro.test"); err != nil {
			t.Fatal(err)
		}
	}
	if updates != 1 {
		t.Fatal("setup was not idempotent")
	}
	for _, event := range subscriptionEvents {
		if !slices.Contains(existing, event) {
			t.Fatalf("missing %s", event)
		}
	}
	if err := ensureSubscriptionWebhook(context.Background(), "https://missing.test"); err == nil {
		t.Fatal("accepted missing webhook")
	}
}

func TestProResumeAndPaymentPortal(t *testing.T) {
	acc := subscriptionFixture(t)
	i := invoiceFixture(acc, "in_pro", time.Now().Add(-time.Hour))
	if err := invoiceGrant(i); err != nil {
		t.Fatal(err)
	}
	remote := stripeSubscription{ID: "sub_fixture", Customer: "cus_fixture", Status: "active", CancelAtEnd: true, PeriodEnd: time.Now().AddDate(0, 1, 0).Unix(), LatestInvoice: i.ID, Metadata: i.SubscriptionDetails.Metadata}
	subscriptions[acc.ID] = subscription{ID: remote.ID, Customer: remote.Customer, Status: remote.Status, CancelAtEnd: true, Cents: 4000, Credits: 4000, AccountCreated: strconv.FormatInt(acc.Created.UnixNano(), 10)}
	oldPortal := paymentPortalConfig
	paymentPortalConfig.ID = ""
	t.Cleanup(func() { paymentPortalConfig = oldPortal })
	configs := 0
	billingHTTP = &http.Client{Transport: stripeTransport(func(r *http.Request) (*http.Response, error) {
		r.ParseForm()
		switch r.URL.Path {
		case "/v1/invoices/in_pro":
			return stripeResponse(i), nil
		case "/v1/subscriptions/sub_fixture":
			if r.Method == "POST" {
				if r.PostForm.Get("cancel_at_period_end") != "false" {
					t.Fatal("resume did not restore renewal")
				}
				remote.CancelAtEnd = false
			}
			return stripeResponse(remote), nil
		case "/v1/billing_portal/configurations":
			configs++
			if r.PostForm.Get("features[payment_method_update][enabled]") != "true" || r.PostForm.Get("features[subscription_update][enabled]") != "false" {
				t.Fatal("portal can bypass plan terms")
			}
			return stripeResponse(map[string]string{"id": "bpc_micro"}), nil
		case "/v1/billing_portal/sessions":
			if r.PostForm.Get("customer") != remote.Customer || r.PostForm.Get("configuration") != "bpc_micro" || r.PostForm.Get("return_url") != "https://micro.test/account#subscription" {
				t.Fatal("portal ownership or return lost")
			}
			return stripeResponse(map[string]string{"url": "https://billing.stripe.com/p/test"}), nil
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
			return nil, nil
		}
	})}
	sess, err := auth.CreateSession(acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/account/subscription", strings.NewReader("action=resume"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
	w := httptest.NewRecorder()
	SubscriptionHandler(w, r)
	if w.Code != 303 || subscriptions[acc.ID].CancelAtEnd || Monthly(acc.ID).Remaining != 4000 {
		t.Fatal("resume changed allowance or failed")
	}
	page := subscriptionSummary(r, acc)
	for _, text := range []string{">Pro<", "4,000 / 4,000", "Renews", "Payment details", "Cancel renewal", "Morning brief"} {
		if !strings.Contains(page, text) {
			t.Fatalf("Account missing %s", text)
		}
	}
	for range 2 {
		if _, err := subscriptionPortal(context.Background(), acc, "https://micro.test"); err != nil {
			t.Fatal(err)
		}
	}
	if configs != 1 {
		t.Fatal("recreated portal for each visit")
	}
	copy := *acc
	copy.Created = copy.Created.Add(time.Second)
	if _, err := subscriptionPortal(context.Background(), &copy, "https://micro.test"); err == nil {
		t.Fatal("reused name obtained former owner's portal")
	}
}
