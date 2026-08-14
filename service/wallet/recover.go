package wallet

// Recovering the signer of a message.
//
// Signing was already here; the other half was not, because until now nothing
// needed to learn who signed something. Verifying an x402 payment is the
// facilitator's job, so this repository only ever produced signatures.
//
// An agent proving who it is without paying needs the reverse: a signature
// arrives and the address has to be derived from it. That derivation is the
// whole security property — only the key holder could have produced a
// signature that recovers to their address — so it is done here in the same
// pure-Go secp256k1 the rest of the wallet uses rather than by adding a
// dependency for one function.

import (
	"fmt"
	"math/big"
)

// ecdsaRecover returns the address that signed hash, given the 65-byte
// [R || S || V] signature.
//
// The standard recovery: R is rebuilt from r and the parity bit, then
// Q = r⁻¹(sR − eG) is the signer's public key.
func ecdsaRecover(hash, sig []byte) (string, error) {
	if len(hash) != 32 {
		return "", fmt.Errorf("hash must be 32 bytes, got %d", len(hash))
	}
	if len(sig) != 65 {
		return "", fmt.Errorf("signature must be 65 bytes, got %d", len(sig))
	}

	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:64])
	v := sig[64]
	// Ethereum writes v as 27/28; some libraries use 0/1. Accept both rather
	// than reject a signature that is correct in the other convention.
	if v >= 27 {
		v -= 27
	}
	if v > 1 {
		return "", fmt.Errorf("bad recovery id %d", sig[64])
	}
	if r.Sign() <= 0 || s.Sign() <= 0 || r.Cmp(secp256k1N) >= 0 || s.Cmp(secp256k1N) >= 0 {
		return "", fmt.Errorf("signature values out of range")
	}

	// R = (r, y), with y chosen to match the parity the signer recorded.
	ry, err := decompressY(r, v)
	if err != nil {
		return "", err
	}

	// Q = r⁻¹ (sR - eG), computed as r⁻¹ (sR + e(-G)).
	e := new(big.Int).SetBytes(hash)
	e.Mod(e, secp256k1N)

	sx, sy := scalarMult(r, ry, s, secp256k1P)

	negE := new(big.Int).Neg(e)
	negE.Mod(negE, secp256k1N)
	ex, ey := scalarMult(secp256k1Gx, secp256k1Gy, negE, secp256k1P)

	qx, qy := pointAdd(sx, sy, ex, ey, secp256k1P)
	rInv := new(big.Int).ModInverse(r, secp256k1N)
	if rInv == nil {
		return "", fmt.Errorf("r has no inverse")
	}
	qx, qy = scalarMult(qx, qy, rInv, secp256k1P)
	if qx == nil || qy == nil {
		return "", fmt.Errorf("recovered point at infinity")
	}

	// The address is the last 20 bytes of the hash of the uncompressed key.
	pub := make([]byte, 64)
	copy(pub[32-len(qx.Bytes()):32], qx.Bytes())
	copy(pub[64-len(qy.Bytes()):64], qy.Bytes())
	return "0x" + toHex(keccak256(pub)[12:]), nil
}

// decompressY solves y² = x³ + 7 for the y whose parity matches want.
//
// secp256k1's p ≡ 3 (mod 4), so the square root is a single exponentiation
// rather than anything iterative.
func decompressY(x *big.Int, want byte) (*big.Int, error) {
	ySq := new(big.Int).Exp(x, big.NewInt(3), secp256k1P)
	ySq.Add(ySq, big.NewInt(7))
	ySq.Mod(ySq, secp256k1P)

	exp := new(big.Int).Add(secp256k1P, big.NewInt(1))
	exp.Div(exp, big.NewInt(4))
	y := new(big.Int).Exp(ySq, exp, secp256k1P)

	// Exp always returns a root; it is only the right one if squaring it comes
	// back to where we started. A bad r has no root at all.
	check := new(big.Int).Mul(y, y)
	check.Mod(check, secp256k1P)
	if check.Cmp(ySq) != 0 {
		return nil, fmt.Errorf("no curve point for r")
	}
	if byte(y.Bit(0)) != want {
		y.Sub(secp256k1P, y)
	}
	return y, nil
}

func toHex(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, c := range b {
		out[i*2] = digits[c>>4]
		out[i*2+1] = digits[c&0x0f]
	}
	return string(out)
}

// signHash produces the 65-byte [R || S || V] signature over a 32-byte hash.
//
// ecdsaSign returns the three parts separately because the x402 payload wants
// them assembled a particular way. Everything else wants the flat Ethereum
// form, and building it in two places is how the two drift.
func signHash(hash, privKey []byte) ([]byte, error) {
	r, s, v, err := ecdsaSign(hash, privKey)
	if err != nil {
		return nil, err
	}
	sig := make([]byte, 65)
	copy(sig[32-len(r.Bytes()):32], r.Bytes())
	copy(sig[64-len(s.Bytes()):64], s.Bytes())
	sig[64] = v + 27
	return sig, nil
}
