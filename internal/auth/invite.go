// Invite-only signup. When enabled (INVITE_ONLY=true), new accounts
// can only be created with a valid invite code. Admins generate invite
// codes via the admin console and the code is emailed to the invitee
// as a signup link.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"mu/internal/data"
)

// Invite stores a pending invitation.
type Invite struct {
	Code      string    `json:"code"`
	Email     string    `json:"email"`     // who it was sent to (informational)
	CreatedBy string    `json:"created_by"` // admin who created it
	CreatedAt time.Time `json:"created_at"`
	UsedBy    string    `json:"used_by,omitempty"` // account ID that consumed it
	UsedAt    time.Time `json:"used_at,omitempty"`
}

var (
	inviteMu sync.Mutex
	invites  = map[string]*Invite{} // code → Invite

	requestMu sync.Mutex
	requests  = map[string]*InviteRequest{} // email → request
)

// InviteRequest is a pending request from someone who wants to join
// on an invite-only instance. Admins review the list and send invites
// to the ones they want to let in.
type InviteRequest struct {
	Email       string    `json:"email"`
	Reason      string    `json:"reason,omitempty"` // optional short message
	IP          string    `json:"ip,omitempty"`
	RequestedAt time.Time `json:"requested_at"`
	Invited     bool      `json:"invited,omitempty"`
	InvitedAt   time.Time `json:"invited_at,omitempty"`
}

func init() {
	b, err := data.LoadFile("invites.json")
	if err == nil && len(b) > 0 {
		var loaded map[string]*Invite
		if err := json.Unmarshal(b, &loaded); err == nil {
			invites = loaded
		}
	}
	b, err = data.LoadFile("invite_requests.json")
	if err == nil && len(b) > 0 {
		var loaded map[string]*InviteRequest
		if err := json.Unmarshal(b, &loaded); err == nil {
			requests = loaded
		}
	}
}

// InviteOnly returns true when signup requires an invite code.
// Controlled by the INVITE_ONLY environment variable.
func InviteOnly() bool {
	v := strings.ToLower(os.Getenv("INVITE_ONLY"))
	return v == "true" || v == "1" || v == "yes"
}

// CreateInvite generates a new invite code for the given email.
// Returns the code. The caller is responsible for emailing it.
func CreateInvite(email, adminID string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	code := hex.EncodeToString(b)

	inviteMu.Lock()
	defer inviteMu.Unlock()

	invites[code] = &Invite{
		Code:      code,
		Email:     email,
		CreatedBy: adminID,
		CreatedAt: time.Now(),
	}
	saveInvites()
	return code, nil
}

// ValidateInvite checks whether a code is valid (exists and unused).
func ValidateInvite(code string) error {
	if code == "" {
		return errors.New("invite code required")
	}
	inviteMu.Lock()
	defer inviteMu.Unlock()

	inv, ok := invites[code]
	if !ok {
		return errors.New("invalid invite code")
	}
	if inv.UsedBy != "" {
		return errors.New("this invite has already been used")
	}
	return nil
}

// ConsumeInvite marks an invite code as used by the given account.
func ConsumeInvite(code, accountID string) {
	inviteMu.Lock()
	defer inviteMu.Unlock()

	if inv, ok := invites[code]; ok {
		inv.UsedBy = accountID
		inv.UsedAt = time.Now()
		saveInvites()
	}
}

// ListInvites returns all invites (for admin console display).
func ListInvites() []*Invite {
	inviteMu.Lock()
	defer inviteMu.Unlock()

	list := make([]*Invite, 0, len(invites))
	for _, inv := range invites {
		list = append(list, inv)
	}
	return list
}

func saveInvites() {
	data.SaveJSON("invites.json", invites)
}

// ============================================================
// Invite requests
// ============================================================

// CreateInviteRequest records that someone wants to join. If an earlier
// request from the same email exists, it's updated in place (so the
// same person can't flood the list with duplicate entries).
func CreateInviteRequest(email, reason, ip string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return errors.New("email is required")
	}
	if len(reason) > 500 {
		reason = reason[:500]
	}
	requestMu.Lock()
	defer requestMu.Unlock()

	now := time.Now()
	if existing, ok := requests[email]; ok {
		// Refresh timestamp so admins see it near the top again, but
		// don't overwrite the invited flag — once sent, stays sent.
		existing.RequestedAt = now
		if reason != "" {
			existing.Reason = reason
		}
		if ip != "" {
			existing.IP = ip
		}
	} else {
		requests[email] = &InviteRequest{
			Email:       email,
			Reason:      reason,
			IP:          ip,
			RequestedAt: now,
		}
	}
	data.SaveJSON("invite_requests.json", requests)
	return nil
}

// ListInviteRequests returns all requests sorted newest first.
// Already-invited requests are at the bottom.
func ListInviteRequests() []*InviteRequest {
	requestMu.Lock()
	defer requestMu.Unlock()

	list := make([]*InviteRequest, 0, len(requests))
	for _, req := range requests {
		list = append(list, req)
	}
	// Pending first, then invited; newest first within each group.
	// Simple bubble is fine — list is tiny.
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			a, b := list[i], list[j]
			if a.Invited == b.Invited {
				if b.RequestedAt.After(a.RequestedAt) {
					list[i], list[j] = b, a
				}
			} else if a.Invited && !b.Invited {
				list[i], list[j] = b, a
			}
		}
	}
	return list
}

// MarkInviteRequestSent flags a request as invited so it drops to the
// bottom of the admin list.
func MarkInviteRequestSent(email string) {
	email = strings.ToLower(strings.TrimSpace(email))
	requestMu.Lock()
	defer requestMu.Unlock()
	if req, ok := requests[email]; ok {
		req.Invited = true
		req.InvitedAt = time.Now()
		data.SaveJSON("invite_requests.json", requests)
	}
}

// DeleteInviteRequest removes a request (admin rejection).
func DeleteInviteRequest(email string) {
	email = strings.ToLower(strings.TrimSpace(email))
	requestMu.Lock()
	defer requestMu.Unlock()
	delete(requests, email)
	data.SaveJSON("invite_requests.json", requests)
}
