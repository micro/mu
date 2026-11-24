package pay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"mu/internal/data"
	"mu/internal/dir"
	"mu/internal/safefetch"
	pb "mu/proto/assistant/v1"
)

type fakeProvider struct {
	calls, credits int
	fail           bool
	keys           []string
}

func (f *fakeProvider) Checkout(_ context.Context, r Record) (string, string, int64, error) {
	f.calls++
	f.keys = append(f.keys, r.Session.Id)
	if f.fail {
		f.fail = false
		return "", "", 0, errors.New("connection lost")
	}
	return "cs_" + r.Session.Id, "https://checkout.stripe.com/c/pay/test", time.Now().Add(24 * time.Hour).Unix(), nil
}
func (f *fakeProvider) Credit(_ context.Context, r Record) error {
	f.credits += int(r.Session.Credits)
	return nil
}
func fixture(t *testing.T) *fakeProvider {
	t.Helper()
	old := Backend
	f := &fakeProvider{}
	Backend = f
	t.Cleanup(func() { Backend = old; _ = os.Remove(filepath.Join(dir.Data(), file)) })
	t.Setenv("MU_PAY_GBP_CREDITS_PER_POUND", "100")
	return f
}
func policy() *Policy {
	return &Policy{Enabled: true, Price: 300, Currency: "gbp", Minutes: 60, ReturnUrl: "https://malten.me/session", NotifyUrl: "https://malten.me/payment"}
}
func checkout(t *testing.T, ref string) *Session {
	t.Helper()
	s, e := Checkout(context.Background(), "owner", "agent", "https://micro.mu", policy(), "secret", &pb.CheckoutRequest{Reference: ref, Participants: 3})
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func payment(s *Session) Settlement {
	return Settlement{ID: s.Id, StripeID: "cs_" + s.Id, Owner: "owner", Agent: "agent", Currency: s.Currency, Status: "paid", Amount: s.Amount}
}
func mutate(t *testing.T, id string, fn func(*Record)) {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()
	r, e := load()
	if e != nil {
		t.Fatal(e)
	}
	v := r[id]
	fn(&v)
	r[id] = v
	if e = data.SaveJSON(file, r); e != nil {
		t.Fatal(e)
	}
}

func TestPurchaseStartRestartAndExpiry(t *testing.T) {
	f := fixture(t)
	s := checkout(t, "booking")
	if s.Amount != 900 || s.Credits != 900 || s.Status != "pending" {
		t.Fatalf("purchase: %+v", s)
	}
	if _, e := Start("owner", "agent", s.Id); !errors.Is(e, ErrInactive) {
		t.Fatal("unpaid start", e)
	}
	for _, args := range [][2]string{{"stranger", "agent"}, {"owner", "other-agent"}} {
		if _, e := Read(args[0], args[1], s.Id); !errors.Is(e, ErrNotFound) {
			t.Fatal("ownership", e)
		}
		if _, e := Start(args[0], args[1], s.Id); !errors.Is(e, ErrNotFound) {
			t.Fatal("start ownership", e)
		}
	}
	unpaid := payment(s)
	unpaid.Status = "unpaid"
	if e := Settle(context.Background(), unpaid); e != nil {
		t.Fatal(e)
	}
	if f.credits != 0 {
		t.Fatal("unpaid credited")
	}
	for _, field := range []string{"amount", "currency", "owner", "agent", "stripe"} {
		p := payment(s)
		switch field {
		case "amount":
			p.Amount++
		case "currency":
			p.Currency = "usd"
		case "owner":
			p.Owner = "stranger"
		case "agent":
			p.Agent = "other"
		case "stripe":
			p.StripeID = "other"
		}
		if e := Settle(context.Background(), p); e == nil {
			t.Fatal("accepted mismatch", field)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := Settle(context.Background(), payment(s)); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if f.credits != 900 {
		t.Fatal("duplicate credit", f.credits)
	}
	paid, e := Read("owner", "agent", s.Id)
	if e != nil || paid.Status != "paid" || paid.EndsAt != 0 {
		t.Fatal("payment started clock", paid, e)
	}
	active, e := Start("owner", "agent", s.Id)
	if e != nil || active.EndsAt-active.StartedAt != 3600 {
		t.Fatal(active, e)
	}
	if e := Bind("owner", "agent", s.Id, "thread"); e != nil {
		t.Fatal(e)
	}
	if e := Check("owner", "agent", s.Id, "thread"); e != nil {
		t.Fatal(e)
	}
	if e := Check("owner", "agent", "", "thread"); !errors.Is(e, ErrInactive) {
		t.Fatal("omission bypass", e)
	}
	if e := Check("owner", "agent", s.Id, "other"); e == nil {
		t.Fatal("second thread accepted")
	}
	// All reads reload the durable file: no in-memory clock or receipt to restore.
	again, e := Start("owner", "agent", s.Id)
	if e != nil || again.EndsAt != active.EndsAt {
		t.Fatal("restart extended session", again, e)
	}
	mutate(t, s.Id, func(r *Record) { r.Session.EndsAt = time.Now().Unix() - 1 })
	expired, e := Start("owner", "agent", s.Id)
	if e != nil || expired.Status != "expired" || expired.EndsAt >= time.Now().Unix() {
		t.Fatal("expired session restarted", expired, e)
	}
	if e := Check("owner", "agent", s.Id, "thread"); !errors.Is(e, ErrInactive) {
		t.Fatal("expiry bypass", e)
	}
	if e := Check("owner", "agent", "", ""); e != nil {
		t.Fatal("ordinary prompt blocked", e)
	}
	next := checkout(t, "renewal")
	if e := Settle(context.Background(), payment(next)); e != nil {
		t.Fatal(e)
	}
	if _, e := Start("owner", "agent", next.Id); e != nil {
		t.Fatal(e)
	}
	if e := Bind("owner", "agent", next.Id, "thread"); e != nil {
		t.Fatal("paid renewal could not continue thread", e)
	}
}
func TestCheckoutRetrySnapshotsPolicy(t *testing.T) {
	f := fixture(t)
	f.fail = true
	q := &pb.CheckoutRequest{Reference: "retry", Participants: 3}
	p := policy()
	if _, e := Checkout(context.Background(), "owner", "agent", "https://micro.mu", p, "secret", q); e == nil {
		t.Fatal("expected lost response")
	}
	p.Price = 500
	p.Minutes = 30
	s, e := Checkout(context.Background(), "owner", "agent", "https://micro.mu", p, "secret", q)
	if e != nil || s.Amount != 900 || s.Minutes != 60 || f.keys[0] != f.keys[1] {
		t.Fatal("retry changed purchase", s, e, f.keys)
	}
	p.Enabled = false
	if _, e = Checkout(context.Background(), "owner", "agent", "https://micro.mu", p, "secret", q); e != nil || f.calls != 2 {
		t.Fatal("retry created another checkout", e)
	}
	q.Participants = 2
	if _, e = Checkout(context.Background(), "owner", "agent", "https://micro.mu", p, "secret", q); e == nil {
		t.Fatal("changed participants")
	}
	mutate(t, s.Id, func(r *Record) { r.Expires = time.Now().Unix() - 1 })
	q.Participants = 3
	if _, e = Checkout(context.Background(), "owner", "agent", "https://micro.mu", p, "secret", q); e == nil {
		t.Fatal("expired idempotency key reused")
	}
}
func TestPolicyAndCorruptStoreFailClosed(t *testing.T) {
	fixture(t)
	t.Setenv("MU_PAY_GBP_CREDITS_PER_POUND", "")
	if credits, e := creditGrant(policy(), 1); e != nil || credits != 300 {
		t.Fatal("default product rate", credits, e)
	}
	t.Setenv("MU_PAY_GBP_CREDITS_PER_POUND", "not-a-rate")
	if _, e := creditGrant(policy(), 1); e == nil {
		t.Fatal("invalid rate accepted")
	}
	for _, raw := range []string{"{", `{"broken":null}`, `{"broken":{"owner":"owner","session":{}}}`} {
		if e := data.SaveFile(file, raw); e != nil {
			t.Fatal(e)
		}
		if _, e := Read("owner", "agent", "id"); e == nil {
			t.Fatal("invalid store accepted", raw)
		}
		if e := Check("owner", "agent", "", "thread"); e == nil {
			t.Fatal("corruption bypass", raw)
		}
	}
}
func TestSignedReceiptRetriesAfterReload(t *testing.T) {
	fixture(t)
	s := checkout(t, "notification")
	if e := Settle(context.Background(), payment(s)); e != nil {
		t.Fatal(e)
	}
	calls := 0
	fetch := func(_ context.Context, url string, o safefetch.Options) (*safefetch.Response, error) {
		calls++
		if url != policy().NotifyUrl || !o.NoRedirect {
			t.Fatal("unsafe notification destination")
		}
		parts := strings.Split(o.Headers["Mu-Pay-Signature"], ",")
		if len(parts) != 2 {
			t.Fatal("signature missing")
		}
		ts := strings.TrimPrefix(parts[0], "t=")
		mac := hmac.New(sha256.New, []byte("secret"))
		mac.Write([]byte(ts + "." + o.Body))
		if parts[1] != "v1="+hex.EncodeToString(mac.Sum(nil)) {
			t.Fatal("invalid signature")
		}
		var body map[string]any
		if e := json.Unmarshal([]byte(o.Body), &body); e != nil || body["reference"] != "notification" || body["participants"] != float64(3) {
			t.Fatal(body, e)
		}
		status := 500
		if calls > 1 {
			status = 204
		}
		return &safefetch.Response{Status: status}, nil
	}
	deliver(context.Background(), fetch)
	deliver(context.Background(), fetch)
	if calls != 1 {
		t.Fatal("failed delivery not backed off")
	}
	mutate(t, s.Id, func(r *Record) { r.NextDelivery = 0 })
	deliver(context.Background(), fetch)
	deliver(context.Background(), fetch)
	if calls != 2 {
		t.Fatal(fmt.Sprint("receipt not acknowledged: ", calls))
	}
}

func TestPublicReturnCannotConfirmPayment(t *testing.T) {
	fixture(t)
	s := checkout(t, "browser-return")
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/pay/"+s.Id, nil))
	if w.Code != 303 || !strings.HasPrefix(w.Header().Get("Location"), "https://checkout.stripe.com/") {
		t.Fatal("checkout redirect", w.Code)
	}
	w = httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/pay/"+s.Id+"?return=1", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "session="+s.Id) {
		t.Fatal("missing app recovery link", w.Code)
	}
	pending, e := Read("owner", "agent", s.Id)
	if e != nil || pending.Status != "pending" {
		t.Fatal("browser return granted access", pending, e)
	}
	if strings.Contains(w.Body.String(), "browser-return") {
		t.Fatal("public page leaked app booking reference")
	}
}

func TestReceiptReservationSurvivesInterruptedDelivery(t *testing.T) {
	fixture(t)
	s := checkout(t, "interrupted")
	if e := Settle(context.Background(), payment(s)); e != nil {
		t.Fatal(e)
	}
	calls := 0
	fetch := func(context.Context, string, safefetch.Options) (*safefetch.Response, error) {
		calls++
		records, e := load()
		if e != nil || records[s.Id].DeliveryState != "sending" || records[s.Id].Attempts != 1 {
			t.Fatal("attempt not reserved before send", e)
		}
		// A second worker sees the reservation and must not deliver in parallel.
		deliver(context.Background(), func(context.Context, string, safefetch.Options) (*safefetch.Response, error) {
			t.Fatal("concurrent duplicate")
			return nil, nil
		})
		panic("process stopped after receiver accepted")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected simulated crash")
			}
		}()
		deliver(context.Background(), fetch)
	}()
	deliver(context.Background(), fetch)
	recovered, e := Read("owner", "agent", s.Id)
	if e != nil || recovered.Status != "paid" || recovered.NotificationStatus != "uncertain" || calls != 1 {
		t.Fatal("ambiguous attempt replayed or booking lost", recovered, e, calls)
	}
}

func TestTransportFailureIsNotReplayed(t *testing.T) {
	fixture(t)
	s := checkout(t, "transport-error")
	if e := Settle(context.Background(), payment(s)); e != nil {
		t.Fatal(e)
	}
	calls := 0
	fetch := func(context.Context, string, safefetch.Options) (*safefetch.Response, error) {
		calls++
		return nil, errors.New("response lost after write")
	}
	deliver(context.Background(), fetch)
	deliver(context.Background(), fetch)
	state, e := Read("owner", "agent", s.Id)
	if e != nil || state.NotificationStatus != "uncertain" || calls != 1 {
		t.Fatal("uncertain transport replayed", state, e, calls)
	}
}
