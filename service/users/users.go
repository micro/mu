// Package users is who is on this instance.
//
// # Why this is a service when internal/user is not
//
// There was a service/user once and it was deleted, correctly. It carried seven
// methods over saving, hiding, flagging and blocking — which read as a service
// because it has a noun and some verbs, but the noun was the caller and the
// verbs changed nothing anybody else could observe. internal/user's package
// comment makes that argument and it still stands: "what have I saved" is a
// question about the asker, which is account furniture rather than a service.
//
// The argument proves less than it was used for. Apply the same test to a
// different question:
//
//	"what have I saved"        — depends on who is asking. Not a service.
//	"who is on this instance"  — does not. A service.
//
// The directory went out with the address book. Nothing answered "who is here",
// which is why a person could sign up alongside a hundred and eighty others and
// meet none of them, and why an agent — on a product whose whole claim is that
// humans and agents share an address space — could not name a single human.
//
// # Users, not people
//
// An agent account is a user of this instance. It signs in, it holds an
// address, it can be written to, and Account.Agent is what distinguishes it.
// "People" would name half the network and quietly exclude the half that makes
// it different from every other network.
//
// # What is published
//
// A projection, never the record. auth.AllAccounts hands out live *Account
// pointers into the map it stores — Secret included — so anything that returned
// those would publish a password hash the first time somebody rendered a
// struct. User below is the public half, written out field by field, and it is
// the only thing that leaves this package.
//
// Place is deliberately absent. It is on the account and it is coarse — a town,
// rounded to about a kilometre — but a hundred and eighty towns in one list is
// a different object from one town on one profile, and a directory is not the
// place to decide that.
package users

import (
	"sort"
	"strings"
	"time"

	"mu/internal/auth"
	"mu/internal/user"
)

// User is somebody here. An Account and a Profile, which are the two halves
// this repository already keeps them in.
//
// internal/auth holds the Account: who you are and how you prove it. internal/user
// holds the Profile: the face you show other people, and whether you are here.
// Its package comment draws exactly that line — "This is not the account" — and
// nothing until now composed the two, so there was no name for the whole person.
// This is the name.
//
// Both halves here are the *public* ones and are written out field by field.
// auth.Account carries Secret; users.Account deliberately does not have the
// field to carry it. A projection that must name everything it publishes fails
// by omitting something harmless; a copy-and-clear fails by publishing a
// password hash, and the line that would have cleared it is invisible in review.
type User struct {
	ID      string  `json:"id" description:"Their username, which is also the local part of their address"`
	Account Account `json:"account" description:"Who they are"`
	Profile Profile `json:"profile" description:"How they appear, and whether they are here"`
}

// Account is the public half of internal/auth.Account.
//
// Not to be confused with it: that one has the secret, the sessions, the place
// and the settings. This is what may be said about somebody to somebody else.
type Account struct {
	Name   string    `json:"name,omitempty" description:"Their display name, when they have set one"`
	Agent  bool      `json:"agent" description:"True when this account is a program rather than a person"`
	Joined time.Time `json:"joined" description:"When the account was created"`
}

// Profile is the public half of internal/user: presence and what they are up to.
type Profile struct {
	Online bool   `json:"online" description:"Seen in the last three minutes"`
	Status string `json:"status,omitempty" description:"What they said they are doing"`
	Page   string `json:"page" description:"The path to their page here"`
}

// Name is what to call them: the display name where there is one, the username
// otherwise. Here rather than at each call site, because "which of these two
// fields do I show" is a question every caller would otherwise answer for
// itself and one of them would answer differently.
func (u User) Display() string {
	if n := strings.TrimSpace(u.Account.Name); n != "" && n != u.ID {
		return n
	}
	return u.ID
}

// List is everyone on this instance, online first and then newest.
//
// Online first because the question behind opening a directory is usually "who
// could answer me now", and a hundred and eighty names sorted by signup date
// answers a different one.
//
// Banned accounts are left out. They are still accounts and an admin can still
// see them at /admin/users; what a directory is for is finding somebody to talk
// to, and the instance has already decided that is not them.
func List() []User {
	online := onlineSet()

	var out []User
	for _, acc := range auth.AllAccounts() {
		if acc == nil || acc.Banned {
			continue
		}
		out = append(out, publicOf(acc, online))
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Profile.Online != out[j].Profile.Online {
			return out[i].Profile.Online
		}
		return out[i].Account.Joined.After(out[j].Account.Joined)
	})
	return out
}

// Get is one user, or false when there is no such account here.
func Get(id string) (User, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return User{}, false
	}
	acc, err := auth.GetAccount(id)
	if err != nil || acc == nil || acc.Banned {
		return User{}, false
	}
	return publicOf(acc, onlineSet()), true
}

// Find is a search over usernames and display names.
//
// Substring rather than the index in internal/data, and deliberately: the
// corpus is the account list, which is already in memory and is small enough
// that indexing it would be machinery guarding nothing. If an instance ever has
// enough accounts for this to matter it will be visible as a slow page, and the
// index is there to move it to.
func Find(query string) []User {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	var out []User
	for _, u := range List() {
		if strings.Contains(strings.ToLower(u.ID), q) ||
			strings.Contains(strings.ToLower(u.Account.Name), q) {
			out = append(out, u)
		}
	}
	return out
}

// Count is how many accounts are on this instance, for a page that wants to say
// so without rendering all of them.
func Count() int { return len(List()) }

// publicOf projects an account into the half that may be published.
//
// Written out field by field rather than by copying the struct and clearing
// what should not go. A copy-and-clear is one forgotten line from publishing a
// secret, and the forgotten line is invisible in review; a projection that has
// to name every field it publishes fails the other way, by omitting something
// harmless.
func publicOf(acc *auth.Account, online map[string]bool) User {
	status, _ := user.Status(acc.ID)
	return User{
		ID: acc.ID,
		Account: Account{
			Name:   acc.Name,
			Agent:  acc.Agent,
			Joined: acc.Created,
		},
		Profile: Profile{
			Online: online[acc.ID],
			Status: status,
			Page:   "/@" + acc.ID,
		},
	}
}

// onlineSet is auth.OnlineUsers as a set, so a list of a hundred and eighty
// does not scan a slice per row.
func onlineSet() map[string]bool {
	out := map[string]bool{}
	for _, id := range auth.OnlineUsers() {
		out[id] = true
	}
	return out
}
