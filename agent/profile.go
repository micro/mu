package agent

import (
	"errors"
	"fmt"
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/pay"
	"net/http"
	"net/url"
	"strings"
)

// profile serves published metadata for guests and non-owners. Owners continue
// into their existing private console. Publishing never grants execution access.
func profile(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/agent/") {
		return false
	}
	name := strings.TrimPrefix(r.URL.Path, "/agent/")
	if name == "" || strings.Contains(name, "/") || reservedName(name) {
		return false
	}
	a, err := namedAgent(name)
	if err != nil {
		http.Error(w, "Agent directory unavailable", http.StatusServiceUnavailable)
		return true
	}
	_, acc := auth.TrySession(r)
	if acc != nil && ((a != nil && acc.ID == a.Owner) || (a == nil && For(acc.ID, name) != nil)) {
		return false
	}
	if a == nil || !a.ProfilePublic {
		app.NotFound(w, r, "Agent not found")
		return true
	}
	w.Header().Set("Cache-Control", "private, no-store")
	body := `<article class="reader-content"><p>` + html.EscapeString(a.Description) + `</p>`
	website, booking := "", ""
	if validPublicURL(a.Website) == nil {
		website = a.Website
	}
	if validPublicURL(a.BookingURL) == nil {
		booking = a.BookingURL
	}
	if website != "" {
		u, _ := url.Parse(website)
		body += `<p><a href="` + html.EscapeString(website) + `" rel="noopener noreferrer">` + html.EscapeString(u.Host) + `</a></p>`
	}
	if a.Pay != nil && a.Pay.Enabled && pay.Validate(a.Pay) == nil {
		body += `<p>` + html.EscapeString("Talk to "+a.Name+" privately · "+sessionPrice(a.Pay)) + `</p>`
		if booking == "" {
			booking = website
		}
		if booking != "" {
			body += `<div class="form-actions"><a class="btn" href="` + html.EscapeString(booking) + `" rel="noopener noreferrer">Book a private session</a></div>`
		}
	}
	if acc == nil && website == "" && booking == "" {
		body += `<div class="form-actions"><a class="btn" href="/login?next=/agent/` + html.EscapeString(Slug(a)) + `">Sign in</a></div>`
	} else if website == "" && booking == "" {
		body += `<p>This is a public profile. Chat access is managed by the agent's owner.</p>`
	}
	body += `</article>`
	app.Respond(w, r, app.Response{Title: a.Name, Description: a.Description, HTML: body})
	return true
}

// Public links are distinct from checkout return/callback URLs and booking IDs.
func validPublicURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || len(raw) > 2048 {
		return errors.New("public links must be HTTPS URLs without credentials or fragments")
	}
	return nil
}
func SetLinks(owner, id, website, booking string) error {
	for _, raw := range []string{website, booking} {
		if err := validPublicURL(raw); err != nil {
			return err
		}
	}
	rosterMutation.Lock()
	defer rosterMutation.Unlock()
	a := For(owner, id)
	if a == nil {
		return errors.New("agent not found")
	}
	a.Website, a.BookingURL = website, booking
	return a.save()
}
func profileLinksForm(owner, id string) string {
	website, booking := "", ""
	if a := For(owner, id); a != nil {
		website, booking = a.Website, a.BookingURL
	}
	return app.Field{ID: "b-website", Label: "Website (public HTTPS address)", Value: website, Wide: true}.HTML() + app.Field{ID: "b-booking", Label: "Booking page (optional; defaults to website)", Value: booking, Wide: true}.HTML()
}
func sessionPrice(p *pay.Policy) string {
	symbol := map[string]string{"gbp": "£", "usd": "$"}[p.Currency]
	price := strings.TrimSuffix(fmt.Sprintf("%.2f", float64(p.Price)/100), ".00")
	duration := fmt.Sprintf("%d minutes", p.Minutes)
	if p.Minutes == 1 {
		duration = "one minute"
	} else if p.Minutes == 60 {
		duration = "one hour"
	} else if p.Minutes%60 == 0 {
		duration = fmt.Sprintf("%d hours", p.Minutes/60)
	}
	return symbol + price + " per person for " + duration
}
