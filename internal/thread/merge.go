package thread

import "fmt"

// resolveUnlocked preserves old links and transport keys without retaining a
// second visible conversation. Every hop remains scoped to the same owner.
func resolveUnlocked(account, id string) *Thread {
	for n := 0; n <= len(owned[account]); n++ {
		t := threads[id]
		if t == nil || t.Account != account {
			return nil
		}
		if t.Canonical == "" {
			return t
		}
		id = t.Canonical
	}
	return nil
}

// Merge moves messages, retaining their IDs, sources and external references.
// copiedRefs identifies obsolete copies by their exact legacy reference, never
// by text. The normalizer runs under the lock and must not call this package.
func Merge(account, sourceID, targetID string, copiedRefs map[string]bool, normalize func(*Message)) error {
	ensure()
	mu.Lock()
	defer mu.Unlock()
	source, target := resolveUnlocked(account, sourceID), resolveUnlocked(account, targetID)
	if source == nil || target == nil {
		return fmt.Errorf("conversation not found")
	}
	if source.ID == target.ID {
		return nil
	}
	keep := make([]*Message, 0, len(messages[source.ID])+len(messages[target.ID]))
	refs := map[string]bool{}
	for _, list := range [][]*Message{messages[source.ID], messages[target.ID]} {
		for _, old := range list {
			if copiedRefs[old.Ref] || (old.Ref != "" && refs[old.Ref]) {
				held[account]--
				continue
			}
			cp := *old
			cp.Thread = target.ID
			if normalize != nil {
				normalize(&cp)
			}
			keep = append(keep, &cp)
			if cp.Ref != "" {
				refs[cp.Ref] = true
			}
		}
	}
	sortByTime(keep)
	messages[target.ID] = keep
	delete(messages, source.ID)
	if source.Updated.After(target.Updated) {
		target.Updated = source.Updated
	}
	if source.Started.Before(target.Started) {
		target.Started = source.Started
	}
	// Keep the older read boundary so merging cannot silently read new arrivals.
	if source.Seen.Before(target.Seen) {
		target.Seen = source.Seen
	}
	for _, p := range source.Parties {
		found := false
		for _, existing := range target.Parties {
			if existing.Kind == p.Kind && existing.Key == p.Key {
				found = true
				break
			}
		}
		if !found {
			target.Parties = append(target.Parties, p)
		}
	}
	source.Canonical = target.ID
	save()
	return nil
}
