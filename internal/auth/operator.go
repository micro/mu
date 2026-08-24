package auth

// Operator is the account that runs this instance.
//
// The first account created becomes admin, and on a self-hosted Mu that is the
// person whose server it is. Anything arriving that belongs to the instance
// rather than to any particular account — a text to the instance's number from
// somebody nobody has a claim on — is theirs to see.
//
// This lived in service/sms as Fallback, which is where it was first needed and
// not where it belongs: it says nothing about phones. service/whatsapp had to
// import service/sms to ask the same question, which is how one service reaching
// into another usually starts — not with something big, with a four-line
// function nobody wanted to write twice.
//
// Empty on an instance with no accounts yet, which is a real state on a first
// run and not an error.
//
// # The oldest admin, not the first one found
//
// This returned whichever admin AllAccounts happened to yield first, and that
// is a map, so on an instance with two admins the answer changed between
// calls. The sentence above says "the first account created", and the code
// said "any of them" — which is fine while there is one and silently wrong the
// day somebody is promoted. A text from a stranger would go to one admin's
// history on Monday and another's on Tuesday, with nothing to say why.
//
// Caught by a test that compared two calls to it, which is the only way an
// unstable answer ever shows up.
func Operator() string {
	var oldest *Account
	for _, acc := range AllAccounts() {
		if acc == nil || !acc.Admin {
			continue
		}
		if oldest == nil || acc.Created.Before(oldest.Created) ||
			(acc.Created.Equal(oldest.Created) && acc.ID < oldest.ID) {
			oldest = acc
		}
	}
	if oldest == nil {
		return ""
	}
	return oldest.ID
}
