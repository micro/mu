package google

// Reading the address book somebody already has.
//
// A lookup, not a copy. The obvious build is to sync Google Contacts into
// service/contacts and be done, and it is the wrong one twice over. Google's
// contacts include everyone the person has ever mailed, so a sync drops
// thousands of junk cards into a list they curate by hand. And it would mean Mu
// holding a copy of their social graph at rest, permanently, whether or not
// they ever ask a question that needs it — which is a lot of standing risk for
// a convenience.
//
// So nothing is stored. A name is resolved against Google when somebody asks,
// and the answer is used and dropped.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ContactsScope is read-only: Mu resolves names, it does not edit anybody's
// address book.
const ContactsScope = "https://www.googleapis.com/auth/contacts.readonly"

// Person is one match from the address book. Deliberately not the contacts
// service's Contact type: this package must not depend on a service, and these
// have no id in Mu because Mu does not hold them.
type Person struct {
	Name  string
	Email string
	Phone string
}

// SearchContacts resolves a name against the person's Google contacts.
//
// searchContacts rather than listing connections, because the question is
// always "who is Sarah" and never "give me everyone" — and the list runs to
// thousands of entries that would be paged through to answer a lookup.
func SearchContacts(accountID, query string, limit int) ([]Person, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	token, err := accessToken(accountID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 10
	}

	// Google's search index for this endpoint is built per session, and the
	// documented way to prime it is a request with an empty query. Without it
	// the first real search can come back empty for no visible reason.
	warm(token)

	q := url.Values{}
	q.Set("query", query)
	q.Set("readMask", "names,emailAddresses,phoneNumbers")
	q.Set("pageSize", fmt.Sprint(limit))

	req, _ := http.NewRequest(http.MethodGet,
		"https://people.googleapis.com/v1/people:searchContacts?"+q.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		// The grant no longer covers this. Forget the scope rather than keep
		// telling somebody they are connected to something that answers nothing.
		dropScope(accountID, ContactsScope)
		return nil, ErrNotConnected
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google contacts search: %s", resp.Status)
	}

	var out struct {
		Results []struct {
			Person struct {
				Names []struct {
					DisplayName string `json:"displayName"`
				} `json:"names"`
				EmailAddresses []struct {
					Value string `json:"value"`
				} `json:"emailAddresses"`
				PhoneNumbers []struct {
					Value string `json:"value"`
				} `json:"phoneNumbers"`
			} `json:"person"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	var people []Person
	for _, r := range out.Results {
		p := Person{}
		if len(r.Person.Names) > 0 {
			p.Name = strings.TrimSpace(r.Person.Names[0].DisplayName)
		}
		if len(r.Person.EmailAddresses) > 0 {
			p.Email = strings.TrimSpace(r.Person.EmailAddresses[0].Value)
		}
		if len(r.Person.PhoneNumbers) > 0 {
			p.Phone = strings.TrimSpace(r.Person.PhoneNumbers[0].Value)
		}
		// A card with a name and no way to reach anybody answers nothing, and
		// resolving a name to an address is the entire job.
		if p.Name == "" || (p.Email == "" && p.Phone == "") {
			continue
		}
		people = append(people, p)
	}
	return people, nil
}

// warm primes the searchContacts index. Failures are ignored: the search that
// follows is the thing that matters, and it reports its own errors.
func warm(token string) {
	q := url.Values{}
	q.Set("query", "")
	q.Set("readMask", "names")
	req, err := http.NewRequest(http.MethodGet,
		"https://people.googleapis.com/v1/people:searchContacts?"+q.Encode(), nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if resp, err := httpClient.Do(req); err == nil {
		resp.Body.Close()
	}
}
