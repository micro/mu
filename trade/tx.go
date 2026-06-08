package trade

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// Transaction represents an EIP-1559 (type 2) transaction.
type Transaction struct {
	ChainID              *big.Int
	Nonce                uint64
	MaxPriorityFeePerGas *big.Int
	MaxFeePerGas         *big.Int
	GasLimit             uint64
	To                   string // hex address
	Value                *big.Int
	Data                 []byte
}

// SignTransaction signs an EIP-1559 transaction and returns the raw
// signed transaction bytes ready for eth_sendRawTransaction.
func SignTransaction(tx *Transaction, privKeyHex string) ([]byte, error) {
	privKey, err := hex.DecodeString(strings.TrimPrefix(privKeyHex, "0x"))
	if err != nil || len(privKey) != 32 {
		return nil, fmt.Errorf("invalid private key")
	}

	encoded := rlpEncodeTx(tx)

	// EIP-1559 signing: hash = keccak256(0x02 || rlp([chainId, nonce, ...]))
	var toHash []byte
	toHash = append(toHash, 0x02)
	toHash = append(toHash, encoded...)
	hash := keccak256(toHash)

	r, s, v, err := ecdsaSign(hash, privKey)
	if err != nil {
		return nil, fmt.Errorf("signing failed: %w", err)
	}

	// Signed tx = 0x02 || rlp([chainId, nonce, maxPriorityFeePerGas, maxFeePerGas, gasLimit, to, value, data, accessList, v, r, s])
	signed := rlpEncodeSignedTx(tx, v, r, s)

	var raw []byte
	raw = append(raw, 0x02)
	raw = append(raw, signed...)
	return raw, nil
}

func rlpEncodeTx(tx *Transaction) []byte {
	toBytes, _ := hex.DecodeString(strings.TrimPrefix(tx.To, "0x"))
	txData := tx.Data
	if txData == nil {
		txData = []byte{}
	}
	items := []any{
		bigIntBytes(tx.ChainID),
		uint64Bytes(tx.Nonce),
		bigIntBytes(tx.MaxPriorityFeePerGas),
		bigIntBytes(tx.MaxFeePerGas),
		uint64Bytes(tx.GasLimit),
		toBytes,
		bigIntBytes(tx.Value),
		txData,
		[]any{}, // accessList (empty)
	}
	return rlpEncodeList(items)
}

func rlpEncodeSignedTx(tx *Transaction, v byte, r, s *big.Int) []byte {
	toBytes, _ := hex.DecodeString(strings.TrimPrefix(tx.To, "0x"))
	txData := tx.Data
	if txData == nil {
		txData = []byte{}
	}
	// yParity: 0 encodes as empty bytes, 1 encodes as [0x01]
	var vBytes []byte
	if v > 0 {
		vBytes = []byte{v}
	}
	items := []any{
		bigIntBytes(tx.ChainID),
		uint64Bytes(tx.Nonce),
		bigIntBytes(tx.MaxPriorityFeePerGas),
		bigIntBytes(tx.MaxFeePerGas),
		uint64Bytes(tx.GasLimit),
		toBytes,
		bigIntBytes(tx.Value),
		txData,
		[]any{},    // accessList
		vBytes,     // yParity
		r.Bytes(),  // r
		s.Bytes(),  // s
	}
	return rlpEncodeList(items)
}

func bigIntBytes(v *big.Int) []byte {
	if v == nil || v.Sign() == 0 {
		return []byte{}
	}
	return v.Bytes()
}

func uint64Bytes(v uint64) []byte {
	if v == 0 {
		return []byte{}
	}
	n := new(big.Int).SetUint64(v)
	return n.Bytes()
}

// ── RLP encoding ──

func rlpEncodeList(items []any) []byte {
	var payload []byte
	for _, item := range items {
		payload = append(payload, rlpEncodeItem(item)...)
	}
	return append(rlpLength(len(payload), 0xc0), payload...)
}

func rlpEncodeItem(v any) []byte {
	switch val := v.(type) {
	case []byte:
		return rlpEncodeBytes(val)
	case []any:
		return rlpEncodeList(val)
	default:
		return rlpEncodeBytes(nil)
	}
}

func rlpEncodeBytes(b []byte) []byte {
	if len(b) == 0 {
		return []byte{0x80}
	}
	if len(b) == 1 && b[0] < 0x80 {
		return b
	}
	return append(rlpLength(len(b), 0x80), b...)
}

func rlpLength(length int, offset byte) []byte {
	if length < 56 {
		return []byte{offset + byte(length)}
	}
	lenBytes := bigIntBytes(new(big.Int).SetInt64(int64(length)))
	return append([]byte{offset + 55 + byte(len(lenBytes))}, lenBytes...)
}

// ── ECDSA signing (secp256k1) ──

var (
	secp256k1N, _    = new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)
	secp256k1HalfN   = new(big.Int).Div(secp256k1N, big.NewInt(2))
	secp256k1Gx, _   = new(big.Int).SetString("79BE667EF9DCBBAC55A06295CE870B07029BFCDB2DCE28D959F2815B16F81798", 16)
	secp256k1Gy, _   = new(big.Int).SetString("483ADA7726A3C4655DA4FBFC0E1108A8FD17B448A68554199C47D08FFB10D4B8", 16)
	secp256k1P, _    = new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEFFFFFC2F", 16)
)

func ecdsaSign(hash, privKey []byte) (r, s *big.Int, v byte, err error) {
	z := new(big.Int).SetBytes(hash)
	d := new(big.Int).SetBytes(privKey)

	k := rfc6979K(privKey, hash)

	// R = k * G
	rx, ry := scalarMult(secp256k1Gx, secp256k1Gy, k, secp256k1P)

	r = new(big.Int).Mod(rx, secp256k1N)
	if r.Sign() == 0 {
		return nil, nil, 0, fmt.Errorf("r is zero")
	}

	// s = k^-1 * (z + r*d) mod n
	kInv := new(big.Int).ModInverse(k, secp256k1N)
	s = new(big.Int).Mul(r, d)
	s.Add(s, z)
	s.Mul(s, kInv)
	s.Mod(s, secp256k1N)

	if s.Sign() == 0 {
		return nil, nil, 0, fmt.Errorf("s is zero")
	}

	// Normalize s to lower half (EIP-2)
	if s.Cmp(secp256k1HalfN) > 0 {
		s.Sub(secp256k1N, s)
		ry.Sub(secp256k1P, ry) // flip y
	}

	// v (yParity): 0 if ry is even, 1 if odd
	v = byte(ry.Bit(0))

	return r, s, v, nil
}

// rfc6979K generates a deterministic k value per RFC 6979.
func rfc6979K(privKey, hash []byte) *big.Int {
	// Step a: h1 = hash (already done)
	// Step b: V = 0x01 * 32
	vv := make([]byte, 32)
	for i := range vv {
		vv[i] = 0x01
	}
	// Step c: K = 0x00 * 32
	kk := make([]byte, 32)

	// Step d: K = HMAC_K(V || 0x00 || privKey || hash)
	kk = hmacSHA256(kk, append(append(append(vv, 0x00), privKey...), hash...))
	// Step e: V = HMAC_K(V)
	vv = hmacSHA256(kk, vv)
	// Step f: K = HMAC_K(V || 0x01 || privKey || hash)
	kk = hmacSHA256(kk, append(append(append(vv, 0x01), privKey...), hash...))
	// Step g: V = HMAC_K(V)
	vv = hmacSHA256(kk, vv)

	// Step h: generate k
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

// ── RPC helpers for transaction submission ──

func getNonce(address string) (uint64, error) {
	url := rpcURL()
	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  "eth_getTransactionCount",
		Params:  []any{address, "pending"},
		ID:      1,
	}
	result, err := doRPC(url, req)
	if err != nil {
		return 0, err
	}
	hexStr := strings.Trim(string(result), `"`)
	val := hexToBigInt(hexStr)
	return val.Uint64(), nil
}

func getGasFees() (maxPriorityFee, maxFee *big.Int, err error) {
	url := rpcURL()

	// Get max priority fee
	req := rpcRequest{JSONRPC: "2.0", Method: "eth_maxPriorityFeePerGas", Params: []any{}, ID: 1}
	result, err := doRPC(url, req)
	if err != nil {
		return nil, nil, err
	}
	maxPriorityFee = hexToBigInt(strings.Trim(string(result), `"`))

	// Get base fee from latest block
	req = rpcRequest{JSONRPC: "2.0", Method: "eth_getBlockByNumber", Params: []any{"latest", false}, ID: 2}
	result, err = doRPC(url, req)
	if err != nil {
		return nil, nil, err
	}
	var block struct {
		BaseFeePerGas string `json:"baseFeePerGas"`
	}
	json.Unmarshal(result, &block)
	baseFee := hexToBigInt(block.BaseFeePerGas)

	// maxFee = 2 * baseFee + maxPriorityFee
	maxFee = new(big.Int).Mul(baseFee, big.NewInt(2))
	maxFee.Add(maxFee, maxPriorityFee)

	return maxPriorityFee, maxFee, nil
}

func estimateGas(from, to string, value *big.Int, data []byte) (uint64, error) {
	url := rpcURL()
	callObj := map[string]string{
		"from": from,
		"to":   to,
		"data": "0x" + hex.EncodeToString(data),
	}
	if value != nil && value.Sign() > 0 {
		callObj["value"] = "0x" + value.Text(16)
	}
	req := rpcRequest{JSONRPC: "2.0", Method: "eth_estimateGas", Params: []any{callObj}, ID: 1}
	result, err := doRPC(url, req)
	if err != nil {
		return 0, err
	}
	val := hexToBigInt(strings.Trim(string(result), `"`))
	// Add 20% buffer
	buffered := new(big.Int).Mul(val, big.NewInt(120))
	buffered.Div(buffered, big.NewInt(100))
	return buffered.Uint64(), nil
}

func sendRawTransaction(signedTx []byte) (string, error) {
	url := rpcURL()
	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  "eth_sendRawTransaction",
		Params:  []any{"0x" + hex.EncodeToString(signedTx)},
		ID:      1,
	}
	result, err := doRPC(url, req)
	if err != nil {
		return "", err
	}
	return strings.Trim(string(result), `"`), nil
}

func waitForReceipt(txHash string) (string, error) {
	url := rpcURL()
	for i := 0; i < 60; i++ {
		req := rpcRequest{JSONRPC: "2.0", Method: "eth_getTransactionReceipt", Params: []any{txHash}, ID: 1}
		result, err := doRPC(url, req)
		if err != nil {
			// RPC error — retry, don't fail immediately
			time.Sleep(2 * time.Second)
			continue
		}
		raw := strings.TrimSpace(string(result))
		if raw == "null" || raw == "" {
			time.Sleep(2 * time.Second)
			continue
		}
		var receipt struct {
			Status  string `json:"status"`
			GasUsed string `json:"gasUsed"`
		}
		if err := json.Unmarshal(result, &receipt); err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if receipt.Status == "0x1" || receipt.Status == "1" {
			return receipt.GasUsed, nil
		}
		if receipt.Status == "0x0" || receipt.Status == "0" {
			return "", fmt.Errorf("transaction reverted")
		}
		return receipt.GasUsed, nil
	}
	return "", fmt.Errorf("receipt timeout after 2 minutes")
}
