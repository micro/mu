// Package contacts is the caller's own address book.
//
// mail could send to an address but nothing could turn a name into one, so
// "email Sarah about Thursday" was not a request Mu could act on — the agent
// either guessed an address or asked a question it should not have needed to
// ask. A name is the way people refer to people; an address is an
// implementation detail they mostly do not remember.
//
// Records live in userdb like every other per-user thing, so ownership,
// listing and the private/public model are the ones the rest of Mu already
// uses. Identity comes from the call context, never a request field.
//
// The store is here rather than in service/contacts because more than one
// service needs to turn a name into a way of reaching somebody. sms asks it
// whether a number is in your address book before it will text a stranger, and
// to reach it, service/sms imported service/contacts — a sideways import over a
// lookup. The service above this one keeps the tools, the page and the
// rendering; what an address book *is* lives here.
//
// The namespace stays "contacts" because that is where every existing record
// already is.
package contacts

import (
	"fmt"
	"sort"
	"strings"

	"mu/internal/app"
	"mu/internal/userdb"
)

const (
	ns         = "contacts"
	collection = "contacts"
)

// Contact is one person in the caller's address book.
type Contact struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
	Note  string `json:"note,omitempty"`
	Owner string `json:"owner"`
}

// Add stores a contact, or updates the one already matching the name.
//
// Re-adding a known name updates it rather than making a second card: an agent
// told "Sarah's new number is X" should not leave two Sarahs behind, and it has
// no way to know whether one already exists.
func Add(owner, name, email, phone, note string) (*Contact, error) {
	if owner == "" {
		return nil, fmt.Errorf("sign in to use contacts")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("a name is required")
	}
	email = strings.TrimSpace(email)
	if email != "" && !strings.Contains(email, "@") {
		return nil, fmt.Errorf("%q does not look like an email address", email)
	}

	fields := map[string]any{"name": name}
	if email != "" {
		fields["email"] = email
	}
	if phone := strings.TrimSpace(phone); phone != "" {
		fields["phone"] = phone
	}
	if note := strings.TrimSpace(note); note != "" {
		fields["note"] = note
	}

	if existing := byExactName(owner, name); existing != nil {
		// Merge: a contact updated with only a phone number keeps its email.
		merged := map[string]any{"name": name}
		if existing.Email != "" {
			merged["email"] = existing.Email
		}
		if existing.Phone != "" {
			merged["phone"] = existing.Phone
		}
		if existing.Note != "" {
			merged["note"] = existing.Note
		}
		for k, v := range fields {
			merged[k] = v
		}
		rec, err := userdb.Update(ns, owner, collection, existing.ID, merged, false)
		if err != nil {
			return nil, err
		}
		return toContact(rec.ID, rec.Owner, rec.Data), nil
	}

	rec, err := userdb.Create(ns, owner, collection, fields, false)
	if err != nil {
		return nil, err
	}
	return toContact(rec.ID, rec.Owner, rec.Data), nil
}

// Find looks a contact up by name, part of a name, or address.
//
// It returns everything that matches rather than a best guess: "Sarah" with two
// Sarahs in the book is a question for the person, not something to resolve by
// picking one and sending mail to it.
func Find(owner, query string) []*Contact {
	query = strings.ToLower(strings.TrimSpace(query))
	if owner == "" || query == "" {
		return nil
	}
	var out []*Contact
	for _, c := range List(owner) {
		if strings.Contains(strings.ToLower(c.Name), query) ||
			strings.Contains(strings.ToLower(c.Email), query) {
			out = append(out, c)
		}
	}
	return out
}

// HasEmail reports whether an address belongs to somebody in this book.
//
// Exact, unlike Find, which matches on substrings so a person can be looked up
// by part of a name. Asked of an address that decides something — may this
// sender wake an agent — a substring match is a hole: "sim@aslam.me" is
// contained in "asim@aslam.me", so a stranger picking a suffix of a contact's
// address would pass.
func HasEmail(owner, email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if owner == "" || email == "" {
		return false
	}
	for _, c := range List(owner) {
		if strings.EqualFold(strings.TrimSpace(c.Email), email) {
			return true
		}
	}
	return false
}

// List returns the caller's contacts, by name.
func List(owner string) []*Contact {
	if owner == "" {
		return nil
	}
	recs, err := userdb.List(ns, owner, collection, "mine", nil, "", "", 0)
	if err != nil {
		return nil
	}
	out := make([]*Contact, 0, len(recs))
	for _, r := range recs {
		out = append(out, toContact(r.ID, r.Owner, r.Data))
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// Remove deletes a contact the caller owns.
func Remove(owner, id string) error {
	if owner == "" {
		return fmt.Errorf("sign in to use contacts")
	}
	return userdb.Delete(ns, owner, collection, strings.TrimSpace(id))
}

// byExactName finds a contact whose name matches exactly, case-insensitively.
func byExactName(owner, name string) *Contact {
	for _, c := range List(owner) {
		if strings.EqualFold(c.Name, name) {
			return c
		}
	}
	return nil
}

func toContact(id, owner string, d map[string]any) *Contact {
	str := func(k string) string { s, _ := d[k].(string); return s }
	return &Contact{
		ID: id, Owner: owner, Name: str("name"),
		Email: str("email"), Phone: str("phone"), Note: str("note"),
	}
}

// Render writes contacts the way a model should read them.
func Render(cs []*Contact) string {
	if len(cs) == 0 {
		return "No matching contacts."
	}
	var b strings.Builder
	for _, c := range cs {
		fmt.Fprintf(&b, "- %s", c.Name)
		if c.Email != "" {
			fmt.Fprintf(&b, " <%s>", c.Email)
		}
		if c.Phone != "" {
			fmt.Fprintf(&b, " tel:%s", c.Phone)
		}
		if c.Note != "" {
			fmt.Fprintf(&b, " — %s", c.Note)
		}
		fmt.Fprintf(&b, " [id: %s]\n", c.ID)
	}
	return strings.TrimSpace(b.String())
}

// DeleteAll removes everything contacts holds for an owner.
//
// Called when the account is deleted (internal/server/hooks.go). Without it
// the records outlived the account that made them: there was no way to ask
// this store for everything one owner had, so the deletion hooks had nothing
// to call and their address book was simply left behind.
func DeleteAll(owner string) {
	if owner == "" {
		return
	}
	if n, err := userdb.DeleteOwner(ns, owner); err != nil {
		app.Log("contacts", "deleting %s's records: %v", owner, err)
	} else if n > 0 {
		app.Log("contacts", "deleted %d records for %s", n, owner)
	}
}
