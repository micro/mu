package mail

import (
	"fmt"
	"sort"
	"strings"

	"mu/internal/auth"
)

// Aliases share one mailbox. They do not create accounts, groups or agents.
func operatorMailbox(local string) (owner, tag string) {
	name, tag := SplitAlias(strings.ToLower(strings.TrimSpace(local)))
	switch name {
	case "admin", "support", "postmaster", "abuse", "security":
		if name != "admin" {
			if tag != "" {
				tag = name + "+" + tag
			} else {
				tag = name
			}
		}
		return auth.OperatorID, tag
	}
	return "", ""
}

func requireOperator(actor string) error {
	a, err := auth.GetAccount(actor)
	if err != nil || !a.Admin || a.System {
		return fmt.Errorf("administrator access required")
	}
	return nil
}

// OperatorMessages returns copies only after checking the human administrator.
func OperatorMessages(actor string) ([]Message, error) {
	if err := requireOperator(actor); err != nil {
		return nil, err
	}
	mutex.RLock()
	defer mutex.RUnlock()
	out := []Message{}
	for _, m := range messages {
		if m.ToID == auth.OperatorID {
			out = append(out, *m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func OperatorMarkRead(actor, id string) error {
	if err := requireOperator(actor); err != nil {
		return err
	}
	return MarkAsRead(id, auth.OperatorID)
}
