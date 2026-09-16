package google

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ContactPage reads one bounded page of the connected address book without copying it.
func ContactPage(owner, page string) ([]Person, string, error) {
	token, err := accessToken(owner)
	if err != nil {
		return nil, "", err
	}
	if len(page) > 4096 {
		return nil, "", fmt.Errorf("invalid page")
	}
	q := url.Values{"personFields": {"names,emailAddresses,phoneNumbers"}, "pageSize": {"50"}, "sortOrder": {"FIRST_NAME_ASCENDING"}}
	if page != "" {
		q.Set("pageToken", page)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://people.googleapis.com/v1/people/me/connections?"+q.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("could not reach Google Contacts")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 403 {
		dropScope(owner, ContactsScope)
		return nil, "", ErrNotConnected
	}
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("Google Contacts returned %d", resp.StatusCode)
	}
	var out struct {
		Connections []struct {
			Names          []struct{ DisplayName string }
			EmailAddresses []struct{ Value string }
			PhoneNumbers   []struct{ Value string }
		}
		NextPageToken string
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&out); err != nil {
		return nil, "", fmt.Errorf("could not read Google Contacts")
	}
	people := make([]Person, 0, len(out.Connections))
	for _, v := range out.Connections {
		p := Person{}
		if len(v.Names) > 0 {
			p.Name = strings.TrimSpace(v.Names[0].DisplayName)
		}
		if len(v.EmailAddresses) > 0 {
			p.Email = v.EmailAddresses[0].Value
		}
		if len(v.PhoneNumbers) > 0 {
			p.Phone = v.PhoneNumbers[0].Value
		}
		if p.Name == "" {
			p.Name = p.Email
		}
		if p.Name == "" {
			p.Name = p.Phone
		}
		if p.Name != "" {
			people = append(people, p)
		}
	}
	return people, out.NextPageToken, nil
}
