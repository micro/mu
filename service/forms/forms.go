// Package forms owns public questionnaires and private responses.
package forms

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/userdb"
)

const namespace = "forms"

var mu sync.Mutex
var fieldName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

type inputError string

func (e inputError) Error() string { return string(e) }

var ErrUnavailable = errors.New("This form is not accepting responses.")

type Field struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}
type Form struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Fields      []Field `json:"fields"`
	Public      bool    `json:"public"`
	Closed      bool    `json:"closed"`
}
type Submission struct {
	ID      string            `json:"id"`
	Created time.Time         `json:"created"`
	Fields  []Field           `json:"fields"`
	Answers map[string]string `json:"answers"`
}

func object(v any) map[string]any {
	b, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}
func decode(r *userdb.Record) (*Form, error) {
	b, err := json.Marshal(r.Data)
	if err != nil {
		return nil, err
	}
	var f Form
	err = json.Unmarshal(b, &f)
	f.ID = r.ID
	f.Public = r.Public
	return &f, err
}
func owned(owner, id string) (*userdb.Record, error) {
	if owner == "" {
		return nil, userdb.ErrAuth
	}
	r, e := userdb.Get(namespace, owner, "forms", id)
	if e != nil {
		return nil, e
	}
	if r.Owner != owner {
		return nil, userdb.ErrNotFound
	}
	return r, nil
}
func validate(f *Form) error {
	f.Title = strings.TrimSpace(f.Title)
	f.Description = strings.TrimSpace(f.Description)
	if f.Title == "" || utf8.RuneCountInString(f.Title) > 160 {
		return errors.New("Enter a title of up to 160 characters.")
	}
	if utf8.RuneCountInString(f.Description) > 2000 {
		return errors.New("Keep the description under 2,000 characters.")
	}
	if len(f.Fields) == 0 || len(f.Fields) > 12 {
		return errors.New("Add between 1 and 12 fields.")
	}
	seen := map[string]bool{}
	for i := range f.Fields {
		v := &f.Fields[i]
		v.Label = strings.TrimSpace(v.Label)
		if !fieldName.MatchString(v.Name) || seen[v.Name] {
			return errors.New("Field names must be unique: lowercase letters, numbers and underscores, starting with a letter.")
		}
		seen[v.Name] = true
		if v.Label == "" || utf8.RuneCountInString(v.Label) > 160 {
			return errors.New("Give each field a label of up to 160 characters.")
		}
		switch v.Type {
		case "text", "textarea", "email":
		default:
			return errors.New("Field types are text, textarea or email.")
		}
	}
	return nil
}

func Save(owner string, f Form) (*Form, error) {
	if owner == "" {
		return nil, userdb.ErrAuth
	}
	if !auth.CanPost(owner) {
		return nil, errors.New(auth.PostBlockReason(owner))
	}
	if err := validate(&f); err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	if f.ID != "" {
		if _, err := owned(owner, f.ID); err != nil {
			return nil, err
		}
	}
	if err := auth.CheckPostRate(owner); err != nil {
		return nil, err
	}
	var r *userdb.Record
	var err error
	if f.ID == "" {
		r, err = userdb.Create(namespace, owner, "forms", object(f), f.Public)
	} else {
		r, err = userdb.Update(namespace, owner, "forms", f.ID, object(f), f.Public)
	}
	if err != nil {
		return nil, err
	}
	return decode(r)
}
func Read(owner, id string) (*Form, error) {
	r, e := owned(owner, id)
	if e != nil {
		return nil, e
	}
	return decode(r)
}
func All(owner string, offset int) ([]*Form, bool, error) {
	if owner == "" {
		return nil, false, userdb.ErrAuth
	}
	rs, more, e := userdb.Page(namespace, owner, "forms", "mine", offset, 50)
	if e != nil {
		return nil, false, e
	}
	out := []*Form{}
	for i := range rs {
		f, e := decode(&rs[i])
		if e != nil {
			return nil, false, e
		}
		out = append(out, f)
	}
	return out, more, nil
}
func public(id string) (*userdb.Record, *Form, error) {
	r, e := userdb.Get(namespace, "", "forms", id)
	if e != nil {
		return nil, nil, ErrUnavailable
	}
	f, e := decode(r)
	if e != nil || !f.Public || f.Closed {
		return nil, nil, ErrUnavailable
	}
	a, e := auth.GetAccount(r.Owner)
	if e != nil || a.Banned {
		return nil, nil, ErrUnavailable
	}
	return r, f, nil
}

// Submit derives ownership from the published form, never from visitor input.
// Submissions are always private, including submissions to public forms.
func Submit(id string, answers map[string]string) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	r, f, e := public(id)
	if e != nil {
		return "", e
	}
	clean := map[string]string{}
	known := map[string]bool{}
	for _, field := range f.Fields {
		known[field.Name] = true
		v := strings.TrimSpace(answers[field.Name])
		limit := 1000
		if field.Type == "textarea" {
			limit = 8000
		}
		if utf8.RuneCountInString(v) > limit {
			return "", inputError(fmt.Sprintf("%s is too long (maximum %d characters).", field.Label, limit))
		}
		if field.Required && v == "" {
			return "", inputError(fmt.Sprintf("%s is required.", field.Label))
		}
		if field.Type == "email" && v != "" {
			a, e := mail.ParseAddress(v)
			if e != nil || a.Address != v {
				return "", inputError(fmt.Sprintf("Enter a valid email address for %s.", field.Label))
			}
		}
		clean[field.Name] = v
	}
	for k := range answers {
		if !known[k] {
			return "", inputError("Unknown form field.")
		}
	}
	submission := Submission{Fields: f.Fields, Answers: clean}
	saved, e := userdb.Create(namespace, r.Owner, "responses-"+f.ID, object(submission), false)
	if e != nil {
		return "", e
	}
	return saved.ID, nil
}
func Responses(owner, id string, offset int) ([]Submission, bool, error) {
	if _, e := owned(owner, id); e != nil {
		return nil, false, e
	}
	rs, more, e := userdb.Page(namespace, owner, "responses-"+id, "mine", offset, 50)
	if e != nil {
		return nil, false, e
	}
	out := []Submission{}
	for _, r := range rs {
		b, _ := json.Marshal(r.Data)
		var s Submission
		if e := json.Unmarshal(b, &s); e != nil {
			return nil, false, e
		}
		s.ID = r.ID
		s.Created = r.Created
		out = append(out, s)
	}
	return out, more, nil
}
func RemoveResponse(owner, id, response string) error {
	mu.Lock()
	defer mu.Unlock()
	if _, e := owned(owner, id); e != nil {
		return e
	}
	return userdb.Delete(namespace, owner, "responses-"+id, response)
}
func Remove(owner, id string) error {
	mu.Lock()
	defer mu.Unlock()
	r, e := owned(owner, id)
	if e != nil {
		return e
	}
	// Unpublish before cleanup, so a failed cleanup cannot accept new responses.
	if _, e = userdb.Update(namespace, owner, "forms", id, r.Data, false); e != nil {
		return e
	}
	if e = userdb.Clear(namespace, owner, "responses-"+id); e != nil {
		return e
	}

	return userdb.Delete(namespace, owner, "forms", id)
}
func DeleteAll(owner string) {
	mu.Lock()
	defer mu.Unlock()
	if _, e := userdb.DeleteOwner(namespace, owner); e != nil {
		app.Log("forms", "account cleanup failed: %v", e)
	}
}
