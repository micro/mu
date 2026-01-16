package mail

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"mu/app"

	"github.com/ProtonMail/go-crypto/openpgp"
)

// decryptPGPMessage attempts to decrypt a PGP encrypted message using native Go openpgp
func decryptPGPMessage(body string) (string, error) {
	// Extract PGP message block
	startMarker := "-----BEGIN PGP MESSAGE-----"
	endMarker := "-----END PGP MESSAGE-----"

	startIdx := strings.Index(body, startMarker)
	endIdx := strings.Index(body, endMarker)

	if startIdx == -1 || endIdx == -1 {
		return "", fmt.Errorf("PGP message markers not found")
	}

	// Extract the full PGP block including markers
	pgpBlock := body[startIdx : endIdx+len(endMarker)]

	// Load private keys from ~/.gnupg or GPG_HOME
	keyring, err := loadPGPKeyring()
	if err != nil {
		return "", fmt.Errorf("failed to load PGP keys: %v", err)
	}

	// Decrypt the message
	md, err := openpgp.ReadMessage(strings.NewReader(pgpBlock), keyring, nil, nil)
	if err != nil {
		if strings.Contains(err.Error(), "private key") {
			return "", fmt.Errorf("no private key found to decrypt this message")
		}
		return "", fmt.Errorf("decryption failed: %v", err)
	}

	// Read decrypted content
	decrypted, err := io.ReadAll(md.UnverifiedBody)
	if err != nil {
		return "", fmt.Errorf("failed to read decrypted message: %v", err)
	}

	if len(decrypted) == 0 {
		return "", fmt.Errorf("decryption produced no output")
	}

	return string(decrypted), nil
}

// loadPGPKeyring loads PGP private keys from the standard GPG directories
func loadPGPKeyring() (openpgp.EntityList, error) {
	// Check if user provided a custom keyring file
	keyringFile := os.Getenv("GPG_KEYRING")
	if keyringFile != "" {
		keyfile, err := os.Open(keyringFile)
		if err != nil {
			return nil, fmt.Errorf("could not open keyring file %s: %v", keyringFile, err)
		}
		defer keyfile.Close()

		keyring, err := openpgp.ReadArmoredKeyRing(keyfile)
		if err != nil {
			return nil, fmt.Errorf("could not read keyring: %v", err)
		}

		if len(keyring) == 0 {
			return nil, fmt.Errorf("no keys found in keyring file")
		}

		app.Log("mail", "Loaded %d PGP keys from %s", len(keyring), keyringFile)
		return keyring, nil
	}

	// Check GPG_HOME environment variable
	gpgHome := os.Getenv("GPG_HOME")
	if gpgHome == "" {
		gpgHome = os.Getenv("GNUPGHOME")
	}
	if gpgHome == "" {
		// Default to ~/.gnupg
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("could not find home directory: %v", err)
		}
		gpgHome = filepath.Join(homeDir, ".gnupg")
	}

	// Try to read secring.gpg (older GPG format)
	secringPath := filepath.Join(gpgHome, "secring.gpg")
	if keyfile, err := os.Open(secringPath); err == nil {
		defer keyfile.Close()
		keyring, err := openpgp.ReadKeyRing(keyfile)
		if err == nil && len(keyring) > 0 {
			app.Log("mail", "Loaded %d PGP keys from %s", len(keyring), secringPath)
			return keyring, nil
		}
	}

	// Try to use gpg command to export keys automatically (modern GPG)
	if _, err := exec.LookPath("gpg"); err == nil {
		app.Log("mail", "Attempting to export PGP keys using gpg command")
		cmd := exec.Command("gpg", "--export-secret-keys", "--armor")
		output, err := cmd.Output()
		if err == nil && len(output) > 0 {
			keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(output))
			if err == nil && len(keyring) > 0 {
				app.Log("mail", "Successfully loaded %d PGP keys from gpg", len(keyring))
				return keyring, nil
			}
		}
	}

	return nil, fmt.Errorf("no PGP keys found - either install gpg with keys or set GPG_KEYRING environment variable")
}
