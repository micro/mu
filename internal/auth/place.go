package auth

// Where an account is, for the code that needs coordinates.
//
// The data lives on Account — see the Place, Lat, Lon and Zone fields — and the
// control for setting it lives in account/, which services may not import. This
// is the read, one level down, so a service can answer for whoever is looking
// without reaching into the account for it.

// Located is an account's coordinates, and whether anybody has said.
//
// A question rather than an instruction, and it hands back only the two numbers
// a tool takes. What a person reads — "Lisbon (Europe/Lisbon)" — is prose for a
// prompt and belongs with the prompt: see account.PlaceLine.
//
// False for an empty account id, which is the signed-out reader, so a caller
// gets one answer to "can I do this for them" rather than two.
func Located(accountID string) (lat, lon float64, ok bool) {
	if accountID == "" {
		return 0, 0, false
	}
	acc, err := GetAccount(accountID)
	if err != nil || acc == nil {
		return 0, 0, false
	}
	if acc.Lat == 0 && acc.Lon == 0 {
		return 0, 0, false
	}
	return acc.Lat, acc.Lon, true
}

// PlaceName is what an account calls where it is, empty when nobody has said.
func PlaceName(accountID string) string {
	if accountID == "" {
		return ""
	}
	acc, err := GetAccount(accountID)
	if err != nil || acc == nil {
		return ""
	}
	return acc.Place
}
