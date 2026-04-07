package mail

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"mu/internal/app"
)

const encPrefix = "enc:" // prefix to identify encrypted fields

var (
	encKey     []byte
	encOnce    sync.Once
	encEnabled bool
)

// initEncryption loads or generates the encryption key
func initEncryption() {
	encOnce.Do(func() {
		// Try env var first
		if keyStr := os.Getenv("MU_ENCRYPTION_KEY"); keyStr != "" {
			decoded, err := base64.StdEncoding.DecodeString(keyStr)
			if err == nil && len(decoded) == 32 {
				encKey = decoded
				encEnabled = true
				app.Log("mail", "Encryption enabled (from MU_ENCRYPTION_KEY)")
				return
			}
			app.Log("mail", "WARNING: MU_ENCRYPTION_KEY invalid (must be 32 bytes, base64-encoded)")
		}

		// Try key file
		keyDir := os.ExpandEnv("$HOME/.mu/keys")
		keyFile := filepath.Join(keyDir, "encryption.key")

		if data, err := os.ReadFile(keyFile); err == nil {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
			if err == nil && len(decoded) == 32 {
				encKey = decoded
				encEnabled = true
				app.Log("mail", "Encryption enabled (from %s)", keyFile)
				return
			}
		}

		// Generate new key
		key := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, key); err != nil {
			app.Log("mail", "WARNING: Failed to generate encryption key: %v", err)
			return
		}

		os.MkdirAll(keyDir, 0700)
		encoded := base64.StdEncoding.EncodeToString(key)
		if err := os.WriteFile(keyFile, []byte(encoded), 0600); err != nil {
			app.Log("mail", "WARNING: Failed to save encryption key: %v", err)
			return
		}

		encKey = key
		encEnabled = true
		app.Log("mail", "Encryption enabled (new key generated at %s)", keyFile)
	})
}

// encrypt encrypts plaintext using AES-256-GCM
func encrypt(plaintext string) (string, error) {
	if !encEnabled || plaintext == "" {
		return plaintext, nil
	}

	block, err := aes.NewCipher(encKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt decrypts ciphertext using AES-256-GCM
func decrypt(ciphertext string) (string, error) {
	if !encEnabled || ciphertext == "" {
		return ciphertext, nil
	}

	// Not encrypted — return as-is (handles pre-encryption messages)
	if !strings.HasPrefix(ciphertext, encPrefix) {
		return ciphertext, nil
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext[len(encPrefix):])
	if err != nil {
		return ciphertext, err
	}

	block, err := aes.NewCipher(encKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// encryptMessage encrypts sensitive fields in a message for storage
func encryptMessage(m *Message) error {
	if !encEnabled {
		return nil
	}

	var err error

	if m.Subject != "" && !strings.HasPrefix(m.Subject, encPrefix) {
		m.Subject, err = encrypt(m.Subject)
		if err != nil {
			return err
		}
	}

	if m.Body != "" && !strings.HasPrefix(m.Body, encPrefix) {
		m.Body, err = encrypt(m.Body)
		if err != nil {
			return err
		}
	}

	if m.To != "" && !strings.HasPrefix(m.To, encPrefix) {
		m.To, err = encrypt(m.To)
		if err != nil {
			return err
		}
	}

	if m.ToID != "" && !strings.HasPrefix(m.ToID, encPrefix) {
		m.ToID, err = encrypt(m.ToID)
		if err != nil {
			return err
		}
	}

	if m.RawHeaders != "" && !strings.HasPrefix(m.RawHeaders, encPrefix) {
		m.RawHeaders, err = encrypt(m.RawHeaders)
		if err != nil {
			return err
		}
	}

	return nil
}

// decryptMessage decrypts sensitive fields in a message after loading
func decryptMessage(m *Message) error {
	if !encEnabled {
		return nil
	}

	var err error

	if strings.HasPrefix(m.Subject, encPrefix) {
		m.Subject, err = decrypt(m.Subject)
		if err != nil {
			return err
		}
	}

	if strings.HasPrefix(m.Body, encPrefix) {
		m.Body, err = decrypt(m.Body)
		if err != nil {
			return err
		}
	}

	if strings.HasPrefix(m.To, encPrefix) {
		m.To, err = decrypt(m.To)
		if err != nil {
			return err
		}
	}

	if strings.HasPrefix(m.ToID, encPrefix) {
		m.ToID, err = decrypt(m.ToID)
		if err != nil {
			return err
		}
	}

	if strings.HasPrefix(m.RawHeaders, encPrefix) {
		m.RawHeaders, err = decrypt(m.RawHeaders)
		if err != nil {
			return err
		}
	}

	return nil
}
