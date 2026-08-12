package auth

// Helpers for tests in other packages that need an account to exist.
//
// internal/quota decides what a caller is charged and cannot answer that
// without an account to look at. Exported rather than duplicated: a test that
// builds its own accounts map is testing its own map.

// SetAccountForTest installs an account directly. Test use only.
func SetAccountForTest(acc *Account) {
	mutex.Lock()
	defer mutex.Unlock()
	accounts[acc.ID] = acc
}

// RemoveAccountForTest drops an account installed by SetAccountForTest.
func RemoveAccountForTest(id string) {
	mutex.Lock()
	defer mutex.Unlock()
	delete(accounts, id)
}
