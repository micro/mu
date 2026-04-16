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
)

func init() {
	b, err := data.LoadFile("invites.json")
	if err == nil && len(b) > 0 {
		var loaded map[string]*Invite
		if err := json.Unmarshal(b, &loaded); err == nil {
			invites = loaded
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
