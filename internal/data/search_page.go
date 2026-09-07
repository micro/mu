package data

import (
	"sort"
	"strings"
)

// searchWords gives retrieval and scoring the same literal terms.
func searchWords(query string) []string {
	words := strings.Fields(strings.ToLower(strings.ReplaceAll(query, "\"", "")))
	result := words[:0]
	for _, word := range words {
		if len(word) >= 2 {
			result = append(result, word)
		}
	}
	return result
}

// SearchIDPage returns at most 200 index IDs, without loading document bodies.
// Ordering is stable for an unchanged index. Owners and types are scoped before
// pagination. Substring fallback applies only when the full-text query has no
// matches anywhere, not when a later page is empty.
func SearchIDPage(query string, limit, offset int, opts ...SearchOption) ([]string, error) {
	words := searchWords(query)
	if len(words) == 0 {
		return nil, nil
	}
	limit = min(max(limit, 1), 200)
	offset = max(offset, 0)
	options := &SearchOptions{}
	for _, opt := range opts {
		opt(options)
	}
	if !UseSQLite {
		indexMutex.RLock()
		defer indexMutex.RUnlock()
		var ids []string
		for id, entry := range index {
			if options.Type != "" && entry.Type != options.Type {
				continue
			}
			if entry.Owner != "" && entry.Owner != options.Owner {
				continue
			}
			title, content := strings.ToLower(entry.Title), strings.ToLower(entry.Content)
			for _, word := range words {
				if strings.Contains(title, word) || strings.Contains(content, word) {
					ids = append(ids, id)
					break
				}
			}
		}
		sort.Strings(ids)
		start := min(offset, len(ids))
		return ids[start : start+min(limit, len(ids)-start)], nil
	}
	db, err := getDB()
	if err != nil {
		return nil, err
	}
	scope := " AND (e.owner = '' OR e.owner = ?)"
	args := []any{buildFTS5Query(words), options.Owner}
	if options.Type != "" {
		scope += " AND e.type = ?"
		args = append(args, options.Type)
	}
	from := " FROM index_fts f JOIN index_entries e ON e.rowid = f.rowid WHERE index_fts MATCH ?" + scope
	var exists bool
	if err := db.QueryRow("SELECT EXISTS(SELECT 1"+from+")", args...).Scan(&exists); err != nil {
		return nil, err
	}
	order := " ORDER BY rank, e.id"
	if !exists {
		var conditions []string
		args = nil
		for _, word := range words {
			// Treat SQL wildcard characters as literal user input.
			word = strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(word)
			conditions = append(conditions, "(LOWER(e.title) LIKE ? ESCAPE '\\' OR LOWER(e.content) LIKE ? ESCAPE '\\')")
			args = append(args, "%"+word+"%", "%"+word+"%")
		}
		from = " FROM index_entries e WHERE (" + strings.Join(conditions, " OR ") + ")" + scope
		args = append(args, options.Owner)
		if options.Type != "" {
			args = append(args, options.Type)
		}
		order = " ORDER BY e.indexed_at DESC, e.id"
	}
	args = append(args, limit, offset)
	rows, err := db.Query("SELECT e.id"+from+order+" LIMIT ? OFFSET ?", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
