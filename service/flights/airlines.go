package flights

// Airline codes, and the translation between the two kinds.
//
// A ticket says BA117. A transponder says BAW117. The first two characters are
// the airline's IATA code, the first three its ICAO code, and there is no rule
// connecting them — BA to BAW looks obvious, U2 to EZY does not, and neither is
// derivable. So: a table.
//
// The translation is usually right and never guaranteed. Most airlines fly a
// flight number under the matching callsign, but a callsign is an air traffic
// control identifier, not a commercial one: some carriers renumber to keep
// similar-sounding callsigns apart on a frequency, and codeshares fly under the
// operating carrier's callsign rather than the one on the ticket. Lookup tries
// the translation first and the raw string after it, so an airline that does its
// own thing is still findable by anyone who knows the callsign.
//
// Sourced from OpenFlights, filtered to active carriers holding both codes.

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
	"unicode"
)

//go:embed airlines.json
var airlinesJSON []byte

// Airline is one carrier under both of its codes.
type Airline struct {
	IATA string `json:"iata"` // two characters, e.g. "BA"
	ICAO string `json:"icao"` // three letters, e.g. "BAW"
	Name string `json:"name"` //
}

var (
	airlinesOnce sync.Once
	airlines     []*Airline
	airlineIATA  map[string]*Airline
	airlineICAO  map[string]*Airline
)

func loadAirlines() {
	airlinesOnce.Do(func() {
		if err := json.Unmarshal(airlinesJSON, &airlines); err != nil {
			return
		}
		airlineIATA = make(map[string]*Airline, len(airlines))
		airlineICAO = make(map[string]*Airline, len(airlines))
		for _, a := range airlines {
			if _, taken := airlineIATA[a.IATA]; !taken {
				airlineIATA[a.IATA] = a
			}
			if _, taken := airlineICAO[a.ICAO]; !taken {
				airlineICAO[a.ICAO] = a
			}
		}
	})
}

// Airlines returns the whole table. Callers must not modify it.
func Airlines() []*Airline {
	loadAirlines()
	return airlines
}

// Callsigns returns the identifiers worth asking the network about, given what
// a caller typed, most likely first.
//
// "BA117" yields BAW117 then BA117; "BAW117" yields only itself, since it is
// already a callsign and no airline holds "BA" as an ICAO code. Padding matters
// too: transponders broadcast a fixed-width field and some airlines fill it, so
// a number is worth trying both bare and with the zeros an airline might add.
func Callsigns(q string) []string {
	loadAirlines()
	q = strings.ToUpper(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '-' {
			return -1
		}
		return r
	}, q))
	if q == "" {
		return nil
	}

	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}

	prefix, number := splitPrefix(q)
	if number != "" {
		// An ICAO prefix is already a callsign; an IATA prefix has to be
		// translated before the network will recognise it.
		if _, ok := airlineICAO[prefix]; ok {
			add(q)
		}
		if a, ok := airlineIATA[prefix]; ok && len(prefix) == 2 {
			add(a.ICAO + number)
			add(a.ICAO + strings.TrimLeft(number, "0"))
			// Do not also try "BA117" as a callsign. Callsign prefixes are
			// three letters, so a two-character one matches nothing — and every
			// candidate here is a second of somebody else's rate limit spent on
			// a question with a known answer.
			return out
		}
	}
	add(q)
	return out
}

// SplitCallsign names the airline behind a callsign, and the flight number it is
// operating. It returns nil when nothing in the table claims the prefix, which
// is the common case for private and military traffic.
func SplitCallsign(cs string) (*Airline, string) {
	loadAirlines()
	prefix, number := splitPrefix(strings.ToUpper(strings.TrimSpace(cs)))
	if number == "" {
		return nil, ""
	}
	if a, ok := airlineICAO[prefix]; ok {
		return a, number
	}
	return nil, ""
}

// splitPrefix cuts an identifier into its leading letters-and-digits airline
// part and the trailing flight number. It splits at the first digit that is
// followed only by digits and at most one letter, which is the shape every
// scheduled callsign has: three letters, some digits, sometimes a suffix.
func splitPrefix(s string) (prefix, number string) {
	for i, r := range s {
		if i == 0 || !unicode.IsDigit(r) {
			continue
		}
		p, n := s[:i], s[i:]
		if len(p) < 2 || len(p) > 3 {
			continue
		}
		if !validNumber(n) {
			continue
		}
		return p, n
	}
	return "", ""
}

// validNumber accepts a flight number: one to four digits, optionally followed
// by a single letter.
func validNumber(n string) bool {
	if n == "" || len(n) > 5 {
		return false
	}
	digits := 0
	for i, r := range n {
		switch {
		case unicode.IsDigit(r):
			if i != digits {
				return false // a digit after the letter
			}
			digits++
		case unicode.IsLetter(r):
			if i != len(n)-1 {
				return false // a letter anywhere but the end
			}
		default:
			return false
		}
	}
	return digits >= 1 && digits <= 4
}

// looksLikeRegistration reports whether a query is shaped like a tail number
// rather than a flight. Registrations are letters and digits with no airline
// prefix — N628TS, G-ZBKL, D-AIMA — and the hyphen most countries use is the
// clearest signal, though the United States does without one.
func looksLikeRegistration(q string) bool {
	q = strings.ToUpper(strings.TrimSpace(q))
	if q == "" || len(q) > 8 {
		return false
	}
	if strings.Contains(q, "-") {
		return true
	}
	if prefix, _ := splitPrefix(q); prefix != "" {
		return false // it parsed as a flight, so read it as one
	}
	for _, r := range q {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// looksLikeHex reports whether a query is an ICAO 24-bit address: exactly six
// hexadecimal characters.
func looksLikeHex(q string) bool {
	q = strings.TrimSpace(q)
	if len(q) != 6 {
		return false
	}
	for _, r := range strings.ToLower(q) {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}
