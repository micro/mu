package wallet

// x402 "exact" scheme payer: sign an EIP-3009 TransferWithAuthorization so the
// user's Base wallet can pay for a resource without the payer submitting a
// transaction (the facilitator broadcasts it, gas-sponsored). This is the
// client side that complements the server side in x402.go.

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
	switch normalizeNetwork(network) {
	case "eip155:8453":
		return 8453, true
	case "eip155:84532":
		return 84532, true
	}
	return 0, false
}

// SignX402Payment builds and signs an EIP-3009 authorization paying the given
// requirement from the wallet, and returns the base64 X-PAYMENT header value.
func SignX402Payment(bw *BaseWallet, req PaymentRequirements) (string, error) {
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
	value, ok := new(big.Int).SetString(strings.TrimSpace(req.MaxAmountRequired), 10)
	if !ok {
		return "", fmt.Errorf("invalid amount %q", req.MaxAmountRequired)
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

	payload := map[string]any{
		"x402Version": x402Version,
		"scheme":      "exact",
		"network":     req.Network,
		"payload": map[string]any{
			"signature":     "0x" + hex.EncodeToString(sig),
			"authorization": auth,
		},
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
