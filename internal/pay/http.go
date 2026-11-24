package pay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/data"
	"mu/internal/safefetch"
)

// Handler exposes only checkout navigation, never booking details or receipts.
// The opaque URL is a checkout capability; it does not grant agent access.
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/pay/")
	mu.Lock()
	records, err := load()
	record, ok := records[id]
	mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if err != nil {
		http.Error(w, "Mu Pay unavailable", 503)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("return") == "1" || record.Session.PaidAt != 0 {
		body := `<p>Your app will confirm your payment. The session starts when its host presses Start.</p><div class="form-actions"><a class="btn" href="` + html.EscapeString(record.ReturnURL) + `">Return to your session</a></div>`
		app.Respond(w, r, app.Response{Title: "Mu Pay", HTML: body})
		return
	}
	if view(record).Status == "expired" {
		http.Error(w, "This checkout has expired", 410)
		return
	}
	u, e := url.Parse(record.StripeURL)
	if e != nil || u.Scheme != "https" || u.Host != "checkout.stripe.com" {
		http.Error(w, "Checkout is being prepared. Please retry shortly.", 503)
		return
	}
	http.Redirect(w, r, record.StripeURL, http.StatusSeeOther)
}

var deliveryOnce sync.Once

// StartDelivery resumes durable receipt delivery after restarts. No polling of
// user accounts or model work; only locally pending payment notifications.
func StartDelivery() {
	deliveryOnce.Do(func() {
		go func() {
			for {
				Deliver(context.Background())
				time.Sleep(30 * time.Second)
			}
		}()
	})
}

// Deliver retries known HTTP failures; uncertain attempts require reconciliation.
// Receivers deduplicate by
// session id; the same id always describes the same payment and purchase.
func Deliver(ctx context.Context) {
	deliver(ctx, safefetch.Fetch)
}

func deliver(ctx context.Context, fetch func(context.Context, string, safefetch.Options) (*safefetch.Response, error)) {
	mu.Lock()
	records, err := load()
	var due []Record
	now := time.Now().Unix()
	if err == nil {
		for _, r := range records {
			if r.Session.PaidAt > 0 && !r.Delivered && r.NotifyURL != "" && r.NextDelivery <= now && (r.DeliveryState == "" || r.DeliveryState == "failed") {
				due = append(due, clone(r))
			}
		}
	}
	mu.Unlock()
	for _, r := range due {
		// Reserve each external attempt durably, rechecking under the lock so
		// concurrent workers cannot dispatch the same receipt. A crash leaves
		// "sending", which is uncertain and must not be replayed automatically.
		mu.Lock()
		currentRecords, loadErr := load()
		current, exists := currentRecords[r.Session.Id]
		if loadErr != nil || !exists || current.Delivered || (current.DeliveryState != "" && current.DeliveryState != "failed") || current.NextDelivery > time.Now().Unix() {
			mu.Unlock()
			continue
		}
		current.Attempts++
		current.DeliveryState = "sending"
		currentRecords[r.Session.Id] = current
		saveErr := data.SaveJSON(file, currentRecords)
		mu.Unlock()
		if saveErr != nil {
			app.Log("pay", "reserve receipt delivery: %v", saveErr)
			continue
		}
		r = clone(current)

		receipt := map[string]any{"id": r.Session.Id, "payment": r.StripeID, "reference": r.Session.Reference, "agent": r.Session.Agent, "participants": r.Session.Participants, "amount": r.Session.Amount, "currency": r.Session.Currency, "credits": r.Session.Credits, "paid_at": r.Session.PaidAt}
		body, _ := json.Marshal(receipt)
		timestamp := fmt.Sprint(time.Now().Unix())
		mac := hmac.New(sha256.New, []byte(r.NotifySecret))
		mac.Write([]byte(timestamp + "." + string(body)))
		signature := "t=" + timestamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))
		response, err := fetch(ctx, r.NotifyURL, safefetch.Options{NoRedirect: true, Method: "POST", Headers: map[string]string{"Content-Type": "application/json", "Mu-Pay-Signature": signature}, Body: string(body), MaxBytes: 4096, Timeout: 10 * time.Second})
		success := err == nil && response != nil && response.Status >= 200 && response.Status < 300
		mu.Lock()
		records, loadErr := load()
		if current, ok := records[r.Session.Id]; loadErr == nil && ok {
			current.Delivered = current.Delivered || success
			switch {
			case success:
				current.DeliveryState = "delivered"
			case err != nil || response == nil:
				current.DeliveryState = "uncertain"
				app.Log("pay", "receipt delivery uncertain for session %s; recover by authenticated session read", r.Session.Id)
			default:
				current.DeliveryState = "failed"
			}
			delay := time.Duration(1<<min(current.Attempts, 10)) * 30 * time.Second
			current.NextDelivery = time.Now().Add(delay).Unix()
			records[r.Session.Id] = current
			if err := data.SaveJSON(file, records); err != nil {
				app.Log("pay", "receipt delivery state: %v", err)
			}
		}
		mu.Unlock()
	}
}
