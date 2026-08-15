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
func Operator() string {
	for _, acc := range AllAccounts() {
		if acc != nil && acc.Admin {
			return acc.ID
		}
	}
	return ""
}
