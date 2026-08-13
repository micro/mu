package wallet

import (
	"os"
	"path/filepath"
	"testing"

	"mu/internal/data"
)

// dataHome points the data package at a scratch directory. dataPath reads $HOME
// on every call, so setting it is enough.
func dataHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".mu", "data"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// The rename must not strand anybody's keys. An instance that has been running
// since before it holds real balances under the old name, and coming up to an
// empty map would mint fresh wallets for those accounts — losing the money in
// the old ones without anything appearing to go wrong.
func TestWalletsAreReadFromTheOldFileAndCopiedForward(t *testing.T) {
	dataHome(t)
	old := map[string]*BaseWallet{
		"acct-1": {Address: "0xaaa", PrivateKey: "11"},
		"acct-2": {Address: "0xbbb", PrivateKey: "22"},
	}
	if err := data.SaveJSON(legacyWalletsFile, old); err != nil {
		t.Fatal(err)
	}

	got := loadWalletsFrom("wallets.json", legacyWalletsFile)
	if len(got) != 2 || got["acct-1"].Address != "0xaaa" {
		t.Fatalf("legacy wallets not read: %+v", got)
	}

	// Copied forward, so the next start does not depend on the old file.
	forward := map[string]*BaseWallet{}
	if err := data.LoadJSON("wallets.json", &forward); err != nil {
		t.Fatalf("new file not written: %v", err)
	}
	if forward["acct-2"].PrivateKey != "22" {
		t.Errorf("key lost in the copy: %+v", forward)
	}
}

// Once the new file exists it is the truth. A stale legacy file must not be
// able to resurrect a wallet that was deleted after the migration.
func TestTheNewFileWinsOverTheOld(t *testing.T) {
	dataHome(t)
	if err := data.SaveJSON(legacyWalletsFile, map[string]*BaseWallet{
		"ghost": {Address: "0xdead", PrivateKey: "99"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := data.SaveJSON("wallets.json", map[string]*BaseWallet{
		"acct-1": {Address: "0xaaa", PrivateKey: "11"},
	}); err != nil {
		t.Fatal(err)
	}

	got := loadWalletsFrom("wallets.json", legacyWalletsFile)
	if _, resurrected := got["ghost"]; resurrected {
		t.Error("a deleted wallet came back from the legacy file")
	}
	if len(got) != 1 {
		t.Errorf("expected only the current file's wallets, got %+v", got)
	}
}

// A fresh instance has neither file and must simply start empty.
func TestNoWalletFilesIsNotAnError(t *testing.T) {
	dataHome(t)
	if got := loadWalletsFrom("wallets.json", legacyWalletsFile); len(got) != 0 {
		t.Errorf("expected no wallets, got %+v", got)
	}
}
