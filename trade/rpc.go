package trade

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"
)

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
	ID      int    `json:"id"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

var rpcClient = &http.Client{Timeout: 15 * time.Second}

func ethCall(to string, data []byte) ([]byte, error) {
	url := rpcURL()
	if url == "" {
		return nil, fmt.Errorf("TRADE_RPC_URL not configured")
	}

	callObj := map[string]string{
		"to":   to,
		"data": "0x" + hex.EncodeToString(data),
	}
	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  "eth_call",
		Params:  []any{callObj, "latest"},
		ID:      1,
	}

	return doRPC(url, req)
}

func ethGetBalance(address string) (*big.Int, error) {
	url := rpcURL()
	if url == "" {
		return nil, fmt.Errorf("TRADE_RPC_URL not configured")
	}

	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  "eth_getBalance",
		Params:  []any{address, "latest"},
		ID:      1,
	}

	result, err := doRPC(url, req)
	if err != nil {
		return nil, err
	}

	return hexToBigInt(strings.Trim(string(result), `"`)), nil
}

func doRPC(url string, req rpcRequest) ([]byte, error) {
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := rpcClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("rpc request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var rpcResp rpcResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("rpc response parse error: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("rpc error: %s", rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

func hexToBigInt(s string) *big.Int {
	s = strings.TrimPrefix(s, "0x")
	val, _ := new(big.Int).SetString(s, 16)
	if val == nil {
		return big.NewInt(0)
	}
	return val
}

// GetETHBalance returns the ETH balance for an address in wei.
func GetETHBalance(address string) (*big.Int, error) {
	return ethGetBalance(address)
}

// GetTokenBalance returns the ERC20 token balance for an address.
func GetTokenBalance(tokenAddress, walletAddress string) (*big.Int, error) {
	// balanceOf(address) = 0x70a08231 + address padded to 32 bytes
	addr := strings.TrimPrefix(strings.ToLower(walletAddress), "0x")
	callData, _ := hex.DecodeString("70a08231" + fmt.Sprintf("%064s", addr))

	result, err := ethCall(tokenAddress, callData)
	if err != nil {
		return nil, err
	}

	hexStr := strings.Trim(string(result), `"`)
	return hexToBigInt(hexStr), nil
}

// GetBalances returns all token balances for a wallet.
func GetBalances(walletAddress string) map[string]string {
	balances := map[string]string{}

	// ETH balance
	if eth, err := GetETHBalance(walletAddress); err == nil {
		balances["ETH"] = FormatAmount(eth, 18)
	}

	// ERC20 balances
	for symbol, token := range Tokens {
		if token.Native {
			continue
		}
		if bal, err := GetTokenBalance(token.Address, walletAddress); err == nil && bal.Sign() > 0 {
			balances[symbol] = FormatAmount(bal, token.Decimals)
		}
	}

	return balances
}
