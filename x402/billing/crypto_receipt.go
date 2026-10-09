package billing

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/sha3"
	"mu/service/wallet"
)

const baseUSDC = "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"

var cryptoHTTP = &http.Client{Timeout: 15 * time.Second}

func cryptoRPC(method string, params any, out any) error {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	resp, err := cryptoHTTP.Post(wallet.BaseRPCURL(), "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("payment node returned %d", resp.StatusCode)
	}
	var result struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&result); err != nil {
		return err
	}
	if len(result.Error) > 0 && string(result.Error) != "null" {
		return fmt.Errorf("payment node rejected %s", method)
	}
	if len(result.Result) == 0 || string(result.Result) == "null" {
		return fmt.Errorf("payment node has no %s result", method)
	}
	return json.Unmarshal(result.Result, out)
}

func cryptoBlock() (uint64, error) {
	var chain, block string
	if err := cryptoRPC("eth_chainId", []any{}, &chain); err != nil {
		return 0, err
	}
	if chain != "0x2105" {
		return 0, fmt.Errorf("payment node is not Base")
	}
	if err := cryptoRPC("eth_blockNumber", []any{}, &block); err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimPrefix(block, "0x"), 16, 64)
}

func cryptoTopic(s string) string {
	h := sha3.NewLegacyKeccak256()
	h.Write([]byte(s))
	return "0x" + hex.EncodeToString(h.Sum(nil))
}
func cryptoAddressTopic(s string) string {
	return "0x" + strings.Repeat("0", 24) + strings.ToLower(strings.TrimPrefix(s, "0x"))
}

type cryptoLog struct {
	Address     string   `json:"address"`
	Topics      []string `json:"topics"`
	Data        string   `json:"data"`
	Transaction string   `json:"transactionHash"`
	Removed     bool     `json:"removed"`
}

// A nonce-use event alone is insufficient: require the successful transaction's
// matching USDC Transfer too. Only finalized blocks recover a missing receipt.
func cryptoReceipt(p cryptoPayment) (string, error) {
	if _, err := cryptoBlock(); err != nil {
		return "", err
	}
	var block struct {
		Number    string `json:"number"`
		Timestamp string `json:"timestamp"`
	}
	if err := cryptoRPC("eth_getBlockByNumber", []any{"finalized", false}, &block); err != nil {
		return "", err
	}
	end, err := strconv.ParseUint(strings.TrimPrefix(block.Number, "0x"), 16, 64)
	if err != nil {
		return "", err
	}
	if end < p.Block {
		return "", nil
	}
	stamp, err := strconv.ParseInt(strings.TrimPrefix(block.Timestamp, "0x"), 16, 64)
	if err != nil {
		return "", err
	}
	if stamp > p.Before {
		// A restart may be weeks later. Bound the log query by the signed
		// expiry rather than asking a node for weeks of chain history.
		lo, hi := p.Block, end
		for lo < hi {
			mid := lo + (hi-lo)/2
			var at struct {
				Timestamp string `json:"timestamp"`
			}
			if err := cryptoRPC("eth_getBlockByNumber", []any{fmt.Sprintf("0x%x", mid), false}, &at); err != nil {
				return "", err
			}
			timestamp, err := strconv.ParseInt(strings.TrimPrefix(at.Timestamp, "0x"), 16, 64)
			if err != nil {
				return "", err
			}
			if timestamp < p.Before {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		block.Number = fmt.Sprintf("0x%x", lo)
	}
	var logs []cryptoLog
	if err := cryptoRPC("eth_getLogs", []any{map[string]any{"address": p.Requirement.Asset, "fromBlock": fmt.Sprintf("0x%x", p.Block), "toBlock": block.Number, "topics": []any{cryptoTopic("AuthorizationUsed(address,bytes32)"), cryptoAddressTopic(p.From), p.Nonce}}}, &logs); err != nil {
		return "", err
	}
	for _, l := range logs {
		if l.Removed || !strings.EqualFold(l.Address, p.Requirement.Asset) || len(l.Topics) != 3 || !strings.EqualFold(l.Topics[0], cryptoTopic("AuthorizationUsed(address,bytes32)")) || !strings.EqualFold(l.Topics[1], cryptoAddressTopic(p.From)) || !strings.EqualFold(l.Topics[2], p.Nonce) || !cryptoHash(l.Transaction) {
			continue
		}
		var receipt struct {
			Status      string      `json:"status"`
			Transaction string      `json:"transactionHash"`
			Logs        []cryptoLog `json:"logs"`
		}
		if err := cryptoRPC("eth_getTransactionReceipt", []any{l.Transaction}, &receipt); err != nil {
			return "", err
		}
		if receipt.Status != "0x1" || !strings.EqualFold(receipt.Transaction, l.Transaction) {
			continue
		}
		for _, transfer := range receipt.Logs {
			if validCryptoTransfer(p, transfer) {
				return strings.ToLower(l.Transaction), nil
			}
		}
	}
	// Once the finalized chain is beyond the authorization window, an absent
	// payment can no longer arrive. It is safe to offer another checkout.
	if stamp > p.Before {
		return "", errCryptoExpired
	}
	return "", nil
}

func validCryptoTransfer(p cryptoPayment, l cryptoLog) bool {
	if l.Removed || !strings.EqualFold(l.Address, p.Requirement.Asset) || len(l.Topics) != 3 || !strings.EqualFold(l.Topics[0], cryptoTopic("Transfer(address,address,uint256)")) || !strings.EqualFold(l.Topics[1], cryptoAddressTopic(p.From)) || !strings.EqualFold(l.Topics[2], cryptoAddressTopic(p.Requirement.PayTo)) {
		return false
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(l.Data, "0x"))
	return err == nil && len(raw) == 32 && strings.EqualFold(l.Data, "0x"+fmt.Sprintf("%064x", creditsAsUSDCAtomic(p.Credits)))
}
