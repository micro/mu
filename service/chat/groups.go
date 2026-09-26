package chat

import (
	"sort"
	"strings"
	"time"

	"mu/internal/event"
	"mu/internal/group"
)

func groupID(room string) string { return strings.TrimPrefix(room, group.RoomPrefix) }
func isGroup(room string) bool   { return strings.HasPrefix(room, group.RoomPrefix) }
func groupMembers(room string) []string {
	g, ok := group.Details(groupID(room))
	if !ok {
		return nil
	}
	out := make([]string, 0, len(g.Members))
	for a := range g.Members {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}
func groupTitle(room string) string {
	g, ok := group.Details(groupID(room))
	if !ok {
		return "Private group"
	}
	return g.Name
}

// Revocation is checked on every read, send and broadcast. This subscription also
// closes idle sockets promptly; the periodic sweep covers a missed bus event.
func watchGroups() {
	sub := event.Subscribe(group.Changed)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		defer sub.Close()
		for {
			select {
			case _, ok := <-sub.Chan:
				if !ok {
					return
				}
			case <-ticker.C:
			}
			pruneGroups()
		}
	}()
}
func pruneGroups() {
	roomsMutex.RLock()
	var active []*Room
	for _, r := range rooms {
		if isGroup(r.ID) {
			active = append(active, r)
		}
	}
	roomsMutex.RUnlock()
	for _, r := range active {
		r.mutex.Lock()
		for conn, c := range r.Clients {
			if !Member(r.ID, c.UserID) {
				delete(r.Clients, conn)
				conn.Close()
			}
		}
		r.mutex.Unlock()
	}
	pruneMUC()
}
