package data

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode"
)

// QueryTerms extracts bounded literal words, never FTS operators. Keep short
// names such as AI and UK; remove question grammar rather than stemming names.
func QueryTerms(query string) []string {
	if len(query) > 2000 {
		query = query[:2000]
	}
	stop := " a an the is are was were be been being do does did can could would should will i me my you your we our it its this that these those what who when where why how tell about please give find show search read summarise summarize explain discuss on in at to for of and or with from as today latest now "
	seen := map[string]bool{}
	var out []string
	for _, word := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if len([]rune(word)) < 2 || strings.Contains(stop, " "+word+" ") || seen[word] {
			continue
		}
		seen[word] = true
		out = append(out, word)
		if len(out) == 8 {
			break
		}
	}
	return out
}

// Passage is an excerpt, not a claim to hold the complete source document.
type Passage struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Text        string `json:"excerpt"`
	URL         string `json:"url,omitempty"`
	Published   string `json:"published,omitempty"`
	ContentKind string `json:"content_kind"`
}

// Retrieve searches only approved public kinds. No LIKE scan or network fallback.
// Context bounds database queueing and search; return/transfer sizes are capped.
func Retrieve(ctx context.Context, terms, kinds []string, since ...time.Time) ([]Passage, error) {
	if len(terms) == 0 || len(kinds) == 0 {
		return nil, nil
	}
	db, err := getDB()
	if err != nil {
		return nil, err
	}
	var quoted []string
	for _, term := range terms {
		quoted = append(quoted, `"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
	}
	marks := make([]string, len(kinds))
	args := []any{strings.Join(quoted, " AND ")}
	for i, k := range kinds {
		marks[i] = "?"
		args = append(args, k)
	}
	dateFilter := ""
	if len(since) > 0 && !since[0].IsZero() {
		dateFilter = " AND CASE WHEN json_valid(e.metadata) THEN julianday(json_extract(e.metadata,'$.posted_at')) >= julianday(?) ELSE 0 END"
		args = append(args, since[0].UTC().Format(time.RFC3339))
	}
	// Explicit public=true is necessary for older blog records whose owner was
	// absent. A missing or malformed visibility flag must fail closed.
	rows, err := db.QueryContext(ctx, `SELECT e.id,e.type,substr(e.title,1,300),
	 snippet(index_fts,1,'','', ' … ',48), substr(e.metadata,1,8000)
	 FROM index_fts JOIN index_entries e ON e.rowid=index_fts.rowid
	 WHERE index_fts MATCH ? AND e.owner='' AND e.type IN (`+strings.Join(marks, ",")+`)
	 AND (e.type!='post' OR CASE WHEN json_valid(e.metadata) THEN json_extract(e.metadata,'$.public')=1 ELSE 0 END)`+dateFilter+`
	 ORDER BY bm25(index_fts,8.0,1.0), CASE WHEN json_valid(e.metadata) THEN julianday(json_extract(e.metadata,'$.posted_at')) END DESC, e.indexed_at DESC LIMIT 5`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Passage
	for rows.Next() {
		var p Passage
		var raw *string
		if err = rows.Scan(&p.ID, &p.Type, &p.Title, &p.Text, &raw); err != nil {
			return nil, err
		}
		p.ContentKind = "archived excerpt; original completeness unknown"
		if raw != nil {
			var m map[string]any
			if json.Unmarshal([]byte(*raw), &m) == nil {
				p.URL, _ = m["url"].(string)
				p.Published, _ = m["posted_at"].(string)
				if kind, ok := m["content_kind"].(string); ok {
					p.ContentKind = kind
				}
			}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
