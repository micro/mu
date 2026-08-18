package push

// The encryption, against the specification's own worked example.
//
// RFC 8291 §5 publishes every intermediate value for one message, which is the
// only honest way to test this: the failure mode of a key derivation is not an
// error, it is a body the browser silently cannot open, and no amount of
// round-tripping against our own code would catch a step done in the wrong
// order — both sides would be wrong together.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"testing"

	"golang.org/x/crypto/hkdf"
)

// The values in RFC 8291 §5, verbatim.
const (
	rfcUAPublic  = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
	rfcUAPrivate = "q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94"
	rfcASPublic  = "BP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8"
	rfcAuth      = "BTBZMqHH6r4Tts7J_aSIgg"
	rfcSalt      = "DGv6ra1nlYgDCS1FRnbzlw"
	rfcPlaintext = "When I grow up, I want to be a watermelon"

	// The derived values the specification publishes, which is what makes this
	// a test rather than a restatement.
	rfcIKM   = "S4lYMb_L0FxCeq0WhDx813KgSYqU26kOyzWUdsXYyrg"
	rfcCEK   = "oIhVW04MRdy2XN9CiKLxTg"
	rfcNonce = "4h_95klXJ5E_qnoN"
)

func mustDecode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := b64.DecodeString(s)
	if err != nil {
		t.Fatalf("could not decode %q: %v", s, err)
	}
	return b
}

// Every derived value, against the ones the specification publishes.
//
// These are the steps that go wrong: an HKDF whose salt and key are the wrong
// way round, a key_info with the two public keys transposed, an extract where an
// expand belongs. None of them errors — each produces a body the browser
// silently cannot open — and a round trip against our own code would not catch
// one, because both halves would be wrong together.
func TestTheDerivationMatchesTheSpecification(t *testing.T) {
	uaPublic := mustDecode(t, rfcUAPublic)
	asPublic := mustDecode(t, rfcASPublic)
	auth := mustDecode(t, rfcAuth)
	salt := mustDecode(t, rfcSalt)

	// The receiver's own private key, so the shared secret is the one the
	// specification computed.
	ua, err := ecdh.P256().NewPrivateKey(mustDecode(t, rfcUAPrivate))
	if err != nil {
		t.Fatal(err)
	}
	if got := b64.EncodeToString(ua.PublicKey().Bytes()); got != rfcUAPublic {
		t.Fatalf("the example's keypair does not agree with itself: %s", got)
	}
	as, err := ecdh.P256().NewPublicKey(asPublic)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := ua.ECDH(as)
	if err != nil {
		t.Fatal(err)
	}

	// The browser's key then ours. Transposed, this derives a different key and
	// reports nothing.
	keyInfo := append([]byte("WebPush: info\x00"), uaPublic...)
	keyInfo = append(keyInfo, asPublic...)
	ikm := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, shared, auth, keyInfo), ikm); err != nil {
		t.Fatal(err)
	}
	if got := b64.EncodeToString(ikm); got != rfcIKM {
		t.Fatalf("IKM is %s, want %s", got, rfcIKM)
	}

	cek := make([]byte, 16)
	if _, err := io.ReadFull(hkdf.New(sha256.New, ikm, salt,
		[]byte("Content-Encoding: aes128gcm\x00")), cek); err != nil {
		t.Fatal(err)
	}
	if got := b64.EncodeToString(cek); got != rfcCEK {
		t.Errorf("CEK is %s, want %s", got, rfcCEK)
	}

	nonce := make([]byte, 12)
	if _, err := io.ReadFull(hkdf.New(sha256.New, ikm, salt,
		[]byte("Content-Encoding: nonce\x00")), nonce); err != nil {
		t.Fatal(err)
	}
	if got := b64.EncodeToString(nonce); got != rfcNonce {
		t.Errorf("NONCE is %s, want %s", got, rfcNonce)
	}

	// And the record encrypts to something the example's own key opens, with
	// the plaintext it says.
	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	sealed := gcm.Seal(nil, nonce, append([]byte(rfcPlaintext), 0x02), nil)
	opened, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(opened[:len(opened)-1]); got != rfcPlaintext {
		t.Errorf("decrypted %q", got)
	}
}

// And the real function's output opens with the browser's key.
//
// The header has to be readable by the receiver without being told anything:
// the salt, the record size and our ephemeral public key are all in it, which is
// what makes the payload self-describing.
func TestWhatWeSendCanBeOpened(t *testing.T) {
	curve := ecdh.P256()
	ua, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := []byte("0123456789abcdef") // 16 bytes, as a subscription's is

	const message = `{"title":"New mail","body":"From a@example.com"}`
	body, err := encrypt(ua.PublicKey().Bytes(), auth, []byte(message))
	if err != nil {
		t.Fatal(err)
	}

	// Read the header back the way a browser does.
	if len(body) < 21 {
		t.Fatalf("the body is %d bytes, too short to hold a header", len(body))
	}
	salt := body[:16]
	if got := binary.BigEndian.Uint32(body[16:20]); got != recordSize {
		t.Errorf("record size is %d, want %d", got, recordSize)
	}
	idlen := int(body[20])
	if idlen != 65 {
		t.Fatalf("the key length byte says %d, want 65", idlen)
	}
	asPublic := body[21 : 21+idlen]
	sealed := body[21+idlen:]

	as, err := curve.NewPublicKey(asPublic)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := ua.ECDH(as)
	if err != nil {
		t.Fatal(err)
	}
	keyInfo := append([]byte("WebPush: info\x00"), ua.PublicKey().Bytes()...)
	keyInfo = append(keyInfo, asPublic...)
	ikm := make([]byte, 32)
	io.ReadFull(hkdf.New(sha256.New, shared, auth, keyInfo), ikm) //nolint:errcheck
	cek := make([]byte, 16)
	io.ReadFull(hkdf.New(sha256.New, ikm, salt, []byte("Content-Encoding: aes128gcm\x00")), cek) //nolint:errcheck
	nonce := make([]byte, 12)
	io.ReadFull(hkdf.New(sha256.New, ikm, salt, []byte("Content-Encoding: nonce\x00")), nonce) //nolint:errcheck

	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		t.Fatalf("the browser could not open it: %v", err)
	}
	// The padding delimiter, and 0x02 rather than 0x01: a receiver told another
	// record follows waits for one that never comes, which fails as a
	// notification that silently never appears.
	if n := len(plain); n == 0 || plain[n-1] != 0x02 {
		t.Fatalf("the record does not end with the last-record delimiter: %x", plain)
	}
	if got := string(plain[:len(plain)-1]); got != message {
		t.Errorf("decrypted %q", got)
	}
}

// A subscription whose auth secret is the wrong length is refused rather than
// producing a body nothing can open.
func TestABadSubscriptionIsRefused(t *testing.T) {
	curve := ecdh.P256()
	ua, _ := curve.GenerateKey(rand.Reader)

	if _, err := encrypt(ua.PublicKey().Bytes(), []byte("short"), []byte("hi")); err == nil {
		t.Error("an eight-byte auth secret was accepted")
	}
	if _, err := encrypt([]byte("not a point"), make([]byte, 16), []byte("hi")); err == nil {
		t.Error("something that is not a P-256 point was accepted as a key")
	}
	big := make([]byte, maxPayload+1)
	if _, err := encrypt(ua.PublicKey().Bytes(), make([]byte, 16), big); err == nil {
		t.Error("a payload too large for one record was accepted")
	}
}
