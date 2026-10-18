package group

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// Scope is an internal resource owner, never an authenticatable user account.
func Scope(id string) string { return "group:" + id }
func FromScope(owner string) (Group, bool) {
	if !strings.HasPrefix(owner, "group:") {
		return Group{}, false
	}
	return Details(strings.TrimPrefix(owner, "group:"))
}
func Admin(id, actor string) bool { r := Role(id, actor); return r == "owner" || r == "admin" }
func RoomGroup(room string) string {
	if !strings.HasPrefix(room, RoomPrefix) {
		return ""
	}
	return strings.SplitN(strings.TrimPrefix(room, RoomPrefix), "_", 2)[0]
}

type Conversation struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Agent   string    `json:"agent"`
	Created time.Time `json:"created"`
}

func Conversations(id, actor string) ([]Conversation, error) {
	g, err := Read(id, actor)
	if err != nil {
		return nil, err
	}
	out := []Conversation{General(g)}
	for _, c := range g.Conversations {
		if c.ID != RoomPrefix+id {
			out = append(out, c)
		}
	}
	return out, nil
}
func General(g Group) Conversation {
	if c, ok := g.Conversations[RoomPrefix+g.ID]; ok {
		return c
	}
	agent := ""
	if g.Encrypted {
		agent = "none"
	}
	return Conversation{ID: RoomPrefix + g.ID, Name: "General", Agent: agent, Created: g.Created}
}
func ConversationFor(room string) (Group, Conversation, bool) {
	g, ok := Details(RoomGroup(room))
	if !ok {
		return Group{}, Conversation{}, false
	}
	if room == RoomPrefix+g.ID {
		return g, General(g), true
	}
	c, ok := g.Conversations[room]
	return g, c, ok
}
func SaveConversation(id, actor, room, name, agent string) (Conversation, error) {
	if err := Load(); err != nil {
		return Conversation{}, err
	}
	mu.Lock()
	defer mu.Unlock()
	g, ok := records[id]
	if !ok || g.Members[actor] == "" {
		return Conversation{}, errors.New("group not found")
	}
	if g.Encrypted && agent != "none" {
		return Conversation{}, errors.New("encrypted conversations cannot use an assistant")
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 100 {
		return Conversation{}, errors.New("choose a conversation name of up to 100 characters")
	}
	c := Conversation{ID: room, Name: name, Agent: agent, Created: time.Now().UTC()}
	if room == "" {
		if len(g.Conversations) >= 100 {
			return c, errors.New("conversation limit reached")
		}
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			return c, err
		}
		c.ID = RoomPrefix + id + "_" + hex.EncodeToString(b[:])
	} else {
		old, exists := g.Conversations[room]
		if room == RoomPrefix+id {
			old, exists = General(g), true
		}
		if !exists || (g.Members[actor] != "owner" && g.Members[actor] != "admin") {
			return c, errors.New("only group admins can change the assistant")
		}
		c.Created = old.Created
	}
	g = clone(g)
	g.Conversations[c.ID] = c
	next := copyRecords()
	next[id] = g
	return c, save(next)
}
func Configure(id, actor string, limit int, membersAI bool) error {
	if limit < 0 || limit > 1000000 {
		return errors.New("choose a monthly limit between 0 and 1,000,000 credits")
	}
	if err := Load(); err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	g, ok := records[id]
	if !ok || (g.Members[actor] != "owner" && g.Members[actor] != "admin") {
		return errors.New("only group admins can change the budget")
	}
	g = clone(g)
	g.SpendLimit, g.MembersUseAI = limit, membersAI
	next := copyRecords()
	next[id] = g
	return save(next)
}
func AllowedAI(g Group, actor string) bool {
	return g.Members[actor] != "" && (g.MembersUseAI || g.Members[actor] != "member")
}

// Invite links carry the secret in a URL fragment; only its hash is stored.
func InvitationLink(id, actor string, revoke bool) (string, error) {
	if err := Load(); err != nil {
		return "", err
	}
	mu.Lock()
	defer mu.Unlock()
	g, ok := records[id]
	if !ok || (g.Members[actor] != "owner" && g.Members[actor] != "admin") {
		return "", errors.New("only group admins can manage invitation links")
	}
	token := ""
	g = clone(g)
	if g.LinkHash != "" {
		g.Links[g.LinkHash] = Invitation{InvitedBy: g.Owner, Expires: g.LinkExpires}
	}
	g.LinkHash, g.LinkExpires = "", time.Time{}
	for hash, inv := range g.Links {
		if time.Now().After(inv.Expires) {
			delete(g.Links, hash)
		}
	}
	if revoke {
		g.Links = map[string]Invitation{}
	}
	if !revoke {
		if len(g.Links) >= 100 {
			return "", errors.New("revoke old invitation links before creating more")
		}
		var b [32]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		token = hex.EncodeToString(b[:])
		sum := sha256.Sum256([]byte(token))
		g.Links[hex.EncodeToString(sum[:])] = Invitation{InvitedBy: actor, Expires: time.Now().UTC().Add(7 * 24 * time.Hour)}
	}
	next := copyRecords()
	next[id] = g
	return token, save(next)
}
func AcceptLink(id, actor, token string) error {
	if actor == "" || token == "" {
		return errors.New("sign in and supply your invitation")
	}
	if err := Load(); err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	g, ok := records[id]
	sum := sha256.Sum256([]byte(token))
	inv, valid := g.Links[hex.EncodeToString(sum[:])]
	if !ok || !valid || time.Now().After(inv.Expires) {
		return errors.New("invitation expired or revoked")
	}
	if g.Members[actor] != "" {
		return nil
	}
	if len(g.Members)+len(g.Invitations) >= 100 {
		return errors.New("this group is full")
	}
	g = clone(g)
	g.Members[actor] = "member"
	delete(g.Invitations, actor)
	next := copyRecords()
	next[id] = g
	return save(next)
}

// ProtectBilling prevents deleting a group while a subscription or checkout
// could still charge its payer. Release it only after payment reconciliation.
func ProtectBilling(id string, until time.Time) error {
	if err := Load(); err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	g, ok := records[id]
	if !ok {
		return errors.New("group not found")
	}
	g = clone(g)
	g.BillingUntil = until
	next := copyRecords()
	next[id] = g
	return save(next)
}
func NoteMessage(id, room string, at time.Time) {
	if Load() != nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	g, ok := records[id]
	if !ok {
		return
	}
	g = clone(g)
	if g.Activity == nil {
		g.Activity = map[string]time.Time{}
	}
	if at.After(g.Activity[room]) {
		g.Activity[room] = at
	}
	next := copyRecords()
	next[id] = g
	_ = save(next)
}
func MarkRead(id, actor, room string) {
	if Load() != nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	g, ok := records[id]
	if !ok || g.Members[actor] == "" {
		return
	}
	g = clone(g)
	if g.Seen == nil {
		g.Seen = map[string]time.Time{}
	}
	g.Seen[actor+"/"+room] = time.Now().UTC()
	next := copyRecords()
	next[id] = g
	_ = save(next)
}
func Unread(g Group, actor string) int {
	n := 0
	for room, at := range g.Activity {
		if at.After(g.Seen[actor+"/"+room]) {
			n++
		}
	}
	return n
}

func Mute(id, actor string, muted bool) error {
	if err := Load(); err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	g, ok := records[id]
	if !ok || g.Members[actor] == "" {
		return errors.New("group not found")
	}
	g = clone(g)
	g.Muted[actor] = muted
	next := copyRecords()
	next[id] = g
	return save(next)
}

// PreviewLink reveals only the invitation's label, never group resources.
func PreviewLink(id, token string) (string, string, error) {
	if err := Load(); err != nil {
		return "", "", err
	}
	mu.RLock()
	defer mu.RUnlock()
	g, ok := records[id]
	sum := sha256.Sum256([]byte(token))
	inv, valid := g.Links[hex.EncodeToString(sum[:])]
	if !ok || !valid || time.Now().After(inv.Expires) {
		return "", "", errors.New("invitation expired or revoked")
	}
	return g.Name, inv.InvitedBy, nil
}
