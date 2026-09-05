package wallet

// x402 "exact" scheme payer: sign an EIP-3009 TransferWithAuthorization so the
// user's Base wallet can pay for a resource without the payer submitting a
// transaction (the facilitator broadcasts it, gas-sponsored). This is the
// client side that complements the server side in internal/x402.

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"mu/internal/x402"
)

// EIP-712 type hashes (keccak256 of the canonical type strings). Verified in
// tests against the well-known constants.
var (
	transferWithAuthorizationTypeHash = keccak256([]byte("TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"))
	eip712DomainTypeHash              = keccak256([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"))
)

// authorization is the EIP-3009 message the payer signs.
type authorization struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Value       string `json:"value"`
	ValidAfter  string `json:"validAfter"`
	ValidBefore string `json:"validBefore"`
	Nonce       string `json:"nonce"`
}

// chainIDFor maps a network id (short or CAIP-2) to its EVM chain id.
func chainIDFor(network string) (int64, bool) {
	switch x402.NormalizeNetwork(network) {
	case "eip155:8453":
		return 8453, true
	case "eip155:84532":
		return 84532, true
	}
	return 0, false
}

// SignX402Payment builds and signs a payment without extension context. It is
// retained for callers that construct requirements directly (for example credit
// top-ups). A resource-server challenge should use SignX402PaymentWithContext so
// v2 resource and extension declarations are echoed as required by the protocol.
func SignX402Payment(bw *BaseWallet, req x402.PaymentRequirements) (string, error) {
	return SignX402PaymentWithContext(bw, req, nil, nil)
}

// SignX402PaymentWithContext builds and signs an EIP-3009 authorization paying
// the given requirement. For x402 v2, resource and extensions are copied from
// the PaymentRequired challenge into PaymentPayload. Extensions are deliberately
// opaque here: the payer must echo declarations it understands without changing
// the server-provided info, and the facilitator decides how to process them.
func SignX402PaymentWithContext(bw *BaseWallet, req x402.PaymentRequirements, resource, extensions map[string]any) (string, error) {
	if bw == nil {
		return "", fmt.Errorf("no wallet")
	}
	chainID, ok := chainIDFor(req.Network)
	if !ok {
		return "", fmt.Errorf("unsupported network %q", req.Network)
	}
	name, version := req.Extra["name"], req.Extra["version"]
	if name == "" || version == "" {
		return "", fmt.Errorf("requirement missing EIP-712 domain (extra.name/version)")
	}
	value, ok := new(big.Int).SetString(req.AmountAtomic(), 10)
	if !ok {
		return "", fmt.Errorf("invalid amount %q", req.AmountAtomic())
	}

	timeout := req.MaxTimeoutSeconds
	if timeout <= 0 {
		timeout = 60
	}
	validBefore := time.Now().Unix() + int64(timeout)

	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	auth := authorization{
		From:        bw.Address,
		To:          req.PayTo,
		Value:       value.String(),
		ValidAfter:  "0",
		ValidBefore: strconv.FormatInt(validBefore, 10),
		Nonce:       "0x" + hex.EncodeToString(nonce),
	}

	digest := eip712Digest(name, version, chainID, req.Asset, auth, value, big.NewInt(0), big.NewInt(validBefore), nonce)

	key, err := hex.DecodeString(strings.TrimPrefix(bw.PrivateKey, "0x"))
	if err != nil || len(key) != 32 {
		return "", fmt.Errorf("bad wallet key")
	}
	r, s, v, err := ecdsaSign(digest, key)
	if err != nil {
		return "", err
	}
	sig := make([]byte, 65)
	copy(sig[32-len(r.Bytes()):32], r.Bytes())
	copy(sig[64-len(s.Bytes()):64], s.Bytes())
	sig[64] = v + 27 // Ethereum signatures use v ∈ {27,28}

	inner := map[string]any{
		"signature":     "0x" + hex.EncodeToString(sig),
		"authorization": auth,
	}

	// The two versions carry the same signature in different envelopes. v1 puts
	// scheme and network beside the payload; v2 replaces both with `accepted` —
	// the whole requirement being paid — so the facilitator can check the payer
	// agreed to the terms it is settling rather than inferring them.
	//
	// Version comes from the challenge, not from this instance's own setting: a
	// payer answers whoever it is calling, and a v1 server must not be sent a v2
	// payload because we happen to advertise v2 ourselves.
	// Told apart by shape rather than by a field: only v2 names the price
	// "amount", so a requirement that has one came from a v2 challenge.
	payloadVersion := 1
	if strings.TrimSpace(req.Amount) != "" {
		payloadVersion = 2
	}

	payload := map[string]any{"x402Version": payloadVersion, "payload": inner}
	if payloadVersion >= 2 {
		payload["accepted"] = req
		if len(resource) > 0 {
			payload["resource"] = resource
		}
		if len(extensions) > 0 {
			payload["extensions"] = extensions
		}
	} else {
		payload["scheme"] = "exact"
		payload["network"] = req.Network
	}
	b, _ := json.Marshal(payload)
	return base64.StdEncoding.EncodeToString(b), nil
}

// eip712Digest computes keccak256(0x1901 || domainSeparator || hashStruct).
func eip712Digest(name, version string, chainID int64, verifyingContract string, auth authorization, value, validAfter, validBefore *big.Int, nonce []byte) []byte {
	domainSeparator := keccak256(
		eip712DomainTypeHash,
		keccak256([]byte(name)),
		keccak256([]byte(version)),
		leftPad32(big.NewInt(chainID).Bytes()),
		leftPad32(addrBytes(verifyingContract)),
	)
	structHash := keccak256(
		transferWithAuthorizationTypeHash,
		leftPad32(addrBytes(auth.From)),
		leftPad32(addrBytes(auth.To)),
		leftPad32(value.Bytes()),
		leftPad32(validAfter.Bytes()),
		leftPad32(validBefore.Bytes()),
		nonce, // already 32 bytes
	)
	return keccak256([]byte{0x19, 0x01}, domainSeparator, structHash)
}

func addrBytes(addr string) []byte {
	b, _ := hex.DecodeString(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(addr)), "0x"))
	return b
}

// leftPad32 left-pads (or right-trims) a byte slice to 32 bytes.
func leftPad32(b []byte) []byte {
	if len(b) >= 32 {
		return b[len(b)-32:]
	}
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out
}
