package push

// Encrypting a notification so the push service cannot read it.
//
// RFC 8291, which is RFC 8188's aes128gcm content encoding with the key agreed
// against the browser's own subscription key. The payload here is the subject
// line of somebody's mail and it travels through Google's or Apple's
// infrastructure; they forward bytes and learn nothing.
//
// The shape, once, because the specification states it as a sequence of key
// derivations and it is easy to get one of them subtly wrong:
//
//	ecdh     = ECDH(our ephemeral private, the browser's public)
//	IKM      = HKDF(salt: auth secret, key: ecdh, info: "WebPush: info"||0||ua||as, 32)
//	PRK      = HKDF-Extract(random 16-byte salt, IKM)
//	CEK      = HKDF-Expand(PRK, "Content-Encoding: aes128gcm"||0, 16)
//	NONCE    = HKDF-Expand(PRK, "Content-Encoding: nonce"||0, 12)
//	body     = salt || record size || key length || our public key || AES-GCM(CEK, NONCE, plaintext||0x02)
//
// The 0x02 is the padding delimiter saying this is the last record. 0x01 means
// another follows, and a receiver given 0x01 on the only record waits for one
// that never comes — which fails as a notification that silently never appears.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

// recordSize is the record size declared in the header. One record is enough
// for a notification; this is the ceiling, and 4096 is what every client
// implementation uses.
const recordSize = 4096

// maxPayload is the largest plaintext that fits one record, allowing for the
// 16-byte GCM tag and the one-byte delimiter. A push service is entitled to
// reject anything larger, so the caller's text is trimmed before it gets here.
const maxPayload = recordSize - 16 - 1 - 86

// encrypt seals a payload for one subscription.
//
// p256dh is the browser's public key and auth its shared secret, both exactly
// as the subscription reported them.
func encrypt(p256dh, auth, plaintext []byte) ([]byte, error) {
	if len(auth) != 16 {
		return nil, fmt.Errorf("the subscription's auth secret is %d bytes, want 16", len(auth))
	}
	if len(plaintext) > maxPayload {
		return nil, fmt.Errorf("payload is %d bytes, over the %d that fit one record",
			len(plaintext), maxPayload)
	}

	curve := ecdh.P256()
	ua, err := curve.NewPublicKey(p256dh)
	if err != nil {
		return nil, fmt.Errorf("the subscription's key is not a P-256 point: %v", err)
	}
	as, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	shared, err := as.ECDH(ua)
	if err != nil {
		return nil, err
	}

	// The browser's key and ours, in that order. Swapping them derives a
	// different key that decrypts to nothing and reports no error here.
	keyInfo := append([]byte("WebPush: info\x00"), p256dh...)
	keyInfo = append(keyInfo, as.PublicKey().Bytes()...)

	ikm := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, shared, auth, keyInfo), ikm); err != nil {
		return nil, err
	}

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}

	cek := make([]byte, 16)
	if _, err := io.ReadFull(hkdf.New(sha256.New, ikm, salt,
		[]byte("Content-Encoding: aes128gcm\x00")), cek); err != nil {
		return nil, err
	}
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(hkdf.New(sha256.New, ikm, salt,
		[]byte("Content-Encoding: nonce\x00")), nonce); err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	// 0x02: the last record. See the note above.
	record := append(append([]byte(nil), plaintext...), 0x02)
	sealed := gcm.Seal(nil, nonce, record, nil)

	pub := as.PublicKey().Bytes()
	body := make([]byte, 0, 16+4+1+len(pub)+len(sealed))
	body = append(body, salt...)
	body = binary.BigEndian.AppendUint32(body, recordSize)
	body = append(body, byte(len(pub)))
	body = append(body, pub...)
	body = append(body, sealed...)
	return body, nil
}
