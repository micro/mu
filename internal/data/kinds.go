package data

// What is in the archive, by kind.
//
// Six services write to this index — news, video, markets, blog, prayer,
// social — and between them they have put a large number of things in it over a
// long time. Nothing could say how many, or of what: the only readers were
// filtered to one type each, so the store was a thing you could ask about news
// and could not ask about itself.
//
// That is the question a page has to answer before it can be a page, and it is
// the question anybody deciding what to keep has to answer first.

import "sort"

// Kind is one type of thing in the archive and how much of it there is.
type Kind struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Kinds is what has been archived, largest first.
//
// Public entries only. A per-account count would be a different question and a
// worse one to answer here — an entry with an owner is somebody's private
// record, and how many of them exist is not archive statistics.
func Kinds() []Kind {
	counts := map[string]int{}

	if UseSQLite {
		db, err := getDB()
		if err != nil {
			return nil
		}
		rows, err := db.Query(`SELECT type, COUNT(*) FROM index_entries WHERE owner = '' GROUP BY type`)
		if err != nil {
			return nil
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			var n int
			if err := rows.Scan(&name, &n); err != nil {
				continue
			}
			counts[name] = n
		}
	} else {
		indexMutex.RLock()
		for _, e := range index {
			if e.Owner != "" {
				continue
			}
			counts[e.Type]++
		}
		indexMutex.RUnlock()
	}

	out := make([]Kind, 0, len(counts))
	for name, n := range counts {
		if name == "" {
			continue
		}
		out = append(out, Kind{Name: name, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}
