package google

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// Calendar is a readable calendar on the connected Google account.
type Calendar struct {
	ID              string `json:"id"`
	Summary         string `json:"summary"`
	SummaryOverride string `json:"summaryOverride"`
	Primary         bool   `json:"primary"`
}

// SelectedCalendars returns a copy of this account's selection. Existing grants
// keep their primary calendar until the person explicitly changes the selection.
func SelectedCalendars(accountID string) []string {
	mu.RLock()
	defer mu.RUnlock()
	c := conns[accountID]
	if c == nil || c.Calendars == nil {
		return []string{"primary"}
	}
	return append([]string{}, c.Calendars...)
}

// Calendars lists all readable calendars, including those hidden in Google's UI.
func Calendars(accountID string) ([]Calendar, error) {
	token, err := accessToken(accountID)
	if err != nil {
		return nil, err
	}
	q := url.Values{"maxResults": {"250"}, "minAccessRole": {"reader"}, "showHidden": {"true"}}
	var calendars []Calendar
	seen := map[string]bool{}
	for {
		req, _ := http.NewRequest(http.MethodGet, "https://www.googleapis.com/calendar/v3/users/me/calendarList?"+q.Encode(), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("google calendar list: %s", resp.Status)
		}
		var out struct {
			Items         []Calendar `json:"items"`
			NextPageToken string     `json:"nextPageToken"`
		}
		err = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, c := range out.Items {
			if c.ID == "" {
				continue
			}
			if c.SummaryOverride != "" {
				c.Summary = c.SummaryOverride
			}
			if c.Summary == "" {
				c.Summary = c.ID
			}
			calendars = append(calendars, c)
		}
		if out.NextPageToken == "" {
			return calendars, nil
		}
		if seen[out.NextPageToken] {
			return nil, fmt.Errorf("google calendar list repeated a page token")
		}
		seen[out.NextPageToken] = true
		q.Set("pageToken", out.NextPageToken)
	}
}

// SetCalendars validates selections against the caller's own Google grant and
// saves them together. Google free/busy supports up to 50 calendars per query.
func SetCalendars(accountID string, ids []string) error {
	if !HasScope(accountID, CalendarScope) {
		return ErrNotConnected
	}
	if len(ids) > 50 {
		return fmt.Errorf("choose up to 50 calendars")
	}
	mu.RLock()
	connection := conns[accountID]
	mu.RUnlock()
	available, err := Calendars(accountID)
	if err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, c := range available {
		allowed[c.ID] = true
	}
	selected := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if !allowed[id] {
			return fmt.Errorf("a selected calendar is no longer available; reload and try again")
		}
		if !seen[id] {
			selected = append(selected, id)
			seen[id] = true
		}
	}
	mu.Lock()
	defer mu.Unlock()
	c := conns[accountID]
	if c == nil || c != connection {
		return ErrNotConnected
	}
	previous := c.Calendars
	c.Calendars = selected
	if err := save(); err != nil {
		c.Calendars = previous
		return err
	}
	return nil
}
