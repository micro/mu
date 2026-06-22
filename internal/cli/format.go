// Output formatting — pretty-printed JSON by default (with colour when
// writing to a tty) and optional table layout for list-shaped results.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// isTerminal returns true when stdout is an interactive terminal.
// We avoid pulling in the x/term dependency by checking the common
// env signals and the file mode.
func isTerminal() bool {
	if os.Getenv("MU_NO_COLOR") != "" || os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// Format prints a tool response text to w using the given resolved
// config as a preference hint.
func Format(w io.Writer, raw string, cfg *ResolvedConfig) error {
	raw = strings.TrimSpace(raw)

	// Decide format preference.
	pretty := cfg.Pretty
	if cfg.Raw {
		pretty = false
	} else if !cfg.Pretty && !isTerminal() {
		// Piping to another command — stay raw for jq-friendliness.
		pretty = false
	} else {
		pretty = true
	}

	// Non-JSON output is printed as-is.
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		fmt.Fprintln(w, raw)
		return nil
	}

	// Table mode: only works for array-of-objects results.
	if cfg.Table {
		if rows, ok := parsed.([]any); ok {
			if printTable(w, rows) {
				return nil
			}
		}
		if m, ok := parsed.(map[string]any); ok {
			for _, v := range m {
				if rows, ok := v.([]any); ok {
					if printTable(w, rows) {
						return nil
					}
				}
			}
		}
		// Fall through to JSON if the result isn't table-shaped.
	}

	if pretty {
		return prettyJSON(w, parsed)
	}
	// Raw: compact JSON to stdout.
	enc := json.NewEncoder(w)
	return enc.Encode(parsed)
}

// prettyJSON writes an indented, syntax-coloured (when tty) JSON dump.
func prettyJSON(w io.Writer, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	text := strings.TrimRight(buf.String(), "\n")

	if !isTerminal() {
		fmt.Fprintln(w, text)
		return nil
	}
	// Minimal ANSI colour for readability.
	coloured := colouriseJSON(text)
	fmt.Fprintln(w, coloured)
	return nil
}

// colouriseJSON applies very light ANSI colouring to already-indented
// JSON. It is intentionally simple: we only colour keys and string
// values, leaving numbers/bools/nulls in default colour.
func colouriseJSON(s string) string {
	const (
		keyColor = "\033[34m" // blue
		strColor = "\033[32m" // green
		reset    = "\033[0m"
	)
	var out strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '"' {
			// Find the matching quote (handle escapes).
			end := i + 1
			for end < len(s) {
				if s[end] == '\\' {
					end += 2
					continue
				}
				if s[end] == '"' {
					break
				}
				end++
			}
			if end >= len(s) {
				out.WriteString(s[i:])
				break
			}
			lit := s[i : end+1]
			// Distinguish key vs value: a key is followed by ":".
			rest := s[end+1:]
			isKey := strings.HasPrefix(strings.TrimLeft(rest, " \t"), ":")
			if isKey {
				out.WriteString(keyColor)
				out.WriteString(lit)
				out.WriteString(reset)
			} else {
				out.WriteString(strColor)
				out.WriteString(lit)
				out.WriteString(reset)
			}
			i = end + 1
			continue
		}
		out.WriteByte(c)
		i++
	}
	return out.String()
}

// printTable renders a slice of JSON objects as an aligned text table.
// Returns false if the rows aren't uniformly object-shaped.
func printTable(w io.Writer, rows []any) bool {
	if len(rows) == 0 {
		fmt.Fprintln(w, "(empty)")
		return true
	}
	// All rows must be maps.
	records := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		m, ok := r.(map[string]any)
		if !ok {
			return false
		}
		records = append(records, m)
	}

	// Collect column names — preserve the first row's key order, then
	// append any extras from subsequent rows.
	seen := map[string]bool{}
	var cols []string
	for _, k := range sortedKeys(records[0]) {
		if !seen[k] {
			cols = append(cols, k)
			seen[k] = true
		}
	}
	for _, rec := range records[1:] {
		for _, k := range sortedKeys(rec) {
			if !seen[k] {
				cols = append(cols, k)
				seen[k] = true
			}
		}
	}

	// Limit columns to keep the table readable. Skip long content
	// fields that would dominate the layout.
	var trimmed []string
	for _, c := range cols {
		low := strings.ToLower(c)
		if low == "html" || low == "body" || low == "content" {
			continue
		}
		trimmed = append(trimmed, c)
		if len(trimmed) >= 6 {
			break
		}
	}
	if len(trimmed) == 0 {
		trimmed = cols
	}
	cols = trimmed

	// Compute widths.
	widths := make(map[string]int, len(cols))
	for _, c := range cols {
		widths[c] = len(c)
	}
	strRows := make([]map[string]string, len(records))
	for i, rec := range records {
		strRows[i] = map[string]string{}
		for _, c := range cols {
			v := rec[c]
			s := formatCell(v)
			if len(s) > 60 {
				s = s[:57] + "..."
			}
			strRows[i][c] = s
			if len(s) > widths[c] {
				widths[c] = len(s)
			}
		}
	}

	// Header.
	for i, c := range cols {
		if i > 0 {
			fmt.Fprint(w, "  ")
		}
		fmt.Fprintf(w, "%-*s", widths[c], strings.ToUpper(c))
	}
	fmt.Fprintln(w)

	// Rows.
	for _, rec := range strRows {
		for i, c := range cols {
			if i > 0 {
				fmt.Fprint(w, "  ")
			}
			fmt.Fprintf(w, "%-*s", widths[c], rec[c])
		}
		fmt.Fprintln(w)
	}
	return true
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func formatCell(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}
