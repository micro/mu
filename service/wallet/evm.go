package wallet

// Pure-Go EVM key + signing primitives for the per-user Base wallet: secp256k1
// (no external crypto dependency), Keccak-256, deterministic ECDSA (RFC 6979),
// address derivation, and keypair generation. Ported from the retired trade
// package so the wallet owns its own crypto.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"golang.org/x/crypto/sha3"
)

var (
	secp256k1N, _  = new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)
	secp256k1HalfN = new(big.Int).Div(secp256k1N, big.NewInt(2))
	secp256k1Gx, _ = new(big.Int).SetString("79BE667EF9DCBBAC55A06295CE870B07029BFCDB2DCE28D959F2815B16F81798", 16)
	secp256k1Gy, _ = new(big.Int).SetString("483ADA7726A3C4655DA4FBFC0E1108A8FD17B448A68554199C47D08FFB10D4B8", 16)
	secp256k1P, _  = new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEFFFFFC2F", 16)
)

// keccak256 hashes data with Keccak-256 (Ethereum's variant).
func keccak256(data ...[]byte) []byte {
	h := sha3.NewLegacyKeccak256()
	for _, d := range data {
		h.Write(d)
	}
	return h.Sum(nil)
}

// GenerateKeypair creates a random secp256k1 keypair, returning the hex private
// key and the 0x-prefixed Ethereum address.
func GenerateKeypair() (privKeyHex, address string, err error) {
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return "", "", err
	}
	return hex.EncodeToString(key), addressFromPrivateKey(key), nil
}

// addressFromPrivateKey derives the Ethereum address from a 32-byte private key.
func addressFromPrivateKey(privKey []byte) string {
	pub := secp256k1PublicKey(privKey)
	hash := keccak256(pub[1:]) // skip the 0x04 uncompressed prefix
	return "0x" + hex.EncodeToString(hash[12:])
}

// AddressFromPrivateKeyHex derives the address for a hex 32-byte key (0x
// optional). Returns ("", false) if the hex isn't a valid 32-byte key.
func AddressFromPrivateKeyHex(hexKey string) (string, bool) {
	hexKey = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(hexKey), "0x"))
	if len(hexKey) != 64 {
		return "", false
	}
	b, err := hex.DecodeString(hexKey)
	if err != nil {
		return "", false
	}
	return addressFromPrivateKey(b), true
}

// secp256k1PublicKey computes the uncompressed public key (0x04 || X || Y).
func secp256k1PublicKey(privKey []byte) []byte {
	k := new(big.Int).SetBytes(privKey)
	k.Mod(k, secp256k1N)
	rx, ry := scalarMult(secp256k1Gx, secp256k1Gy, k, secp256k1P)
	xBytes, yBytes := rx.Bytes(), ry.Bytes()
	pub := make([]byte, 65)
	pub[0] = 0x04
	copy(pub[1+32-len(xBytes):33], xBytes)
	copy(pub[33+32-len(yBytes):65], yBytes)
	return pub
}

// ecdsaSign produces a deterministic (RFC 6979) secp256k1 signature over hash,
// normalized to low-s (EIP-2), returning r, s and the recovery parity v.
func ecdsaSign(hash, privKey []byte) (r, s *big.Int, v byte, err error) {
	z := new(big.Int).SetBytes(hash)
	d := new(big.Int).SetBytes(privKey)
	k := rfc6979K(privKey, hash)

	rx, ry := scalarMult(secp256k1Gx, secp256k1Gy, k, secp256k1P)
	r = new(big.Int).Mod(rx, secp256k1N)
	if r.Sign() == 0 {
		return nil, nil, 0, fmt.Errorf("r is zero")
	}

	kInv := new(big.Int).ModInverse(k, secp256k1N)
	s = new(big.Int).Mul(r, d)
	s.Add(s, z)
	s.Mul(s, kInv)
	s.Mod(s, secp256k1N)
	if s.Sign() == 0 {
		return nil, nil, 0, fmt.Errorf("s is zero")
	}

	if s.Cmp(secp256k1HalfN) > 0 {
		s.Sub(secp256k1N, s)
		ry.Sub(secp256k1P, ry)
	}
	v = byte(ry.Bit(0))
	return r, s, v, nil
}

// rfc6979K derives the deterministic nonce k per RFC 6979.
func rfc6979K(privKey, hash []byte) *big.Int {
	vv := make([]byte, 32)
	for i := range vv {
		vv[i] = 0x01
	}
	kk := make([]byte, 32)
	kk = hmacSHA256(kk, append(append(append(vv, 0x00), privKey...), hash...))
	vv = hmacSHA256(kk, vv)
	kk = hmacSHA256(kk, append(append(append(vv, 0x01), privKey...), hash...))
	vv = hmacSHA256(kk, vv)
	for {
		vv = hmacSHA256(kk, vv)
		k := new(big.Int).SetBytes(vv)
		if k.Sign() > 0 && k.Cmp(secp256k1N) < 0 {
			return k
		}
		kk = hmacSHA256(kk, append(vv, 0x00))
		vv = hmacSHA256(kk, vv)
	}
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func scalarMult(gx, gy, k, p *big.Int) (*big.Int, *big.Int) {
	rx, ry := new(big.Int), new(big.Int)
	isZero := true
	for i := k.BitLen() - 1; i >= 0; i-- {
		if !isZero {
			rx, ry = pointDouble(rx, ry, p)
		}
		if k.Bit(i) == 1 {
			if isZero {
				rx.Set(gx)
				ry.Set(gy)
				isZero = false
			} else {
				rx, ry = pointAdd(rx, ry, gx, gy, p)
			}
		}
	}
	return rx, ry
}

func pointAdd(x1, y1, x2, y2, p *big.Int) (*big.Int, *big.Int) {
	dy := new(big.Int).Sub(y2, y1)
	dx := new(big.Int).Sub(x2, x1)
	dx.ModInverse(dx, p)
	s := new(big.Int).Mul(dy, dx)
	s.Mod(s, p)
	rx := new(big.Int).Mul(s, s)
	rx.Sub(rx, x1)
	rx.Sub(rx, x2)
	rx.Mod(rx, p)
	ry := new(big.Int).Sub(x1, rx)
	ry.Mul(ry, s)
	ry.Sub(ry, y1)
	ry.Mod(ry, p)
	return rx, ry
}

func pointDouble(x, y, p *big.Int) (*big.Int, *big.Int) {
	three := big.NewInt(3)
	two := big.NewInt(2)
	x2 := new(big.Int).Mul(x, x)
	num := new(big.Int).Mul(three, x2)
	den := new(big.Int).Mul(two, y)
	den.ModInverse(den, p)
	s := new(big.Int).Mul(num, den)
	s.Mod(s, p)
	rx := new(big.Int).Mul(s, s)
	rx.Sub(rx, new(big.Int).Mul(two, x))
	rx.Mod(rx, p)
	ry := new(big.Int).Sub(x, rx)
	ry.Mul(ry, s)
	ry.Sub(ry, y)
	ry.Mod(ry, p)
	return rx, ry
}
