package trade

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"mu/internal/app"
)

// Quote represents a swap price quote.
type Quote struct {
	FromToken   string `json:"from_token"`
	ToToken     string `json:"to_token"`
	AmountIn    string `json:"amount_in"`
	AmountOut   string `json:"amount_out"`
	PriceImpact string `json:"price_impact,omitempty"`
	PoolFee     string `json:"pool_fee"`
}

// GetQuote gets a swap quote from the Uniswap V3 Quoter.
func GetQuote(fromSymbol, toSymbol, amount string) (*Quote, error) {
	from, ok := Tokens[strings.ToUpper(fromSymbol)]
	if !ok {
		return nil, fmt.Errorf("unknown token: %s", fromSymbol)
	}
	to, ok := Tokens[strings.ToUpper(toSymbol)]
	if !ok {
		return nil, fmt.Errorf("unknown token: %s", toSymbol)
	}

	fromAddr := from.Address
	toAddr := to.Address
	if from.Native {
		fromAddr = Tokens["WETH"].Address
	}
	if to.Native {
		toAddr = Tokens["WETH"].Address
	}

	amountIn, err := ParseAmount(amount, from.Decimals)
	if err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}

	// Use Uniswap V3 Quoter: quoteExactInputSingle
	// Function selector: 0xc6a5026a (QuoterV2)
	// Params: (tokenIn, tokenOut, amountIn, fee, sqrtPriceLimitX96)
	fee := big.NewInt(3000) // 0.3% pool
	callData := buildQuoteCalldata(fromAddr, toAddr, amountIn, fee)

	result, err := ethCall(UniswapQuoterAddr, callData)
	if err != nil {
		// Try 0.05% pool (500) for stablecoin pairs
		fee = big.NewInt(500)
		callData = buildQuoteCalldata(fromAddr, toAddr, amountIn, fee)
		result, err = ethCall(UniswapQuoterAddr, callData)
		if err != nil {
			return nil, fmt.Errorf("quote failed: %w", err)
		}
	}

	hexStr := strings.Trim(string(result), `"`)
	hexStr = strings.TrimPrefix(hexStr, "0x")
	if len(hexStr) < 64 {
		return nil, fmt.Errorf("unexpected quote response")
	}
	amountOut := hexToBigInt(hexStr[:64])

	feeStr := "0.3%"
	if fee.Int64() == 500 {
		feeStr = "0.05%"
	}

	return &Quote{
		FromToken: from.Symbol,
		ToToken:   to.Symbol,
		AmountIn:  amount + " " + from.Symbol,
		AmountOut: FormatAmount(amountOut, to.Decimals) + " " + to.Symbol,
		PoolFee:   feeStr,
	}, nil
}

// buildQuoteCalldata builds the calldata for QuoterV2.quoteExactInputSingle.
// Struct param: (address tokenIn, address tokenOut, uint256 amountIn, uint24 fee, uint160 sqrtPriceLimitX96)
func buildQuoteCalldata(tokenIn, tokenOut string, amountIn, fee *big.Int) []byte {
	// quoteExactInputSingle((address,address,uint256,uint24,uint160))
	// selector: 0xc6a5026a
	selector, _ := hex.DecodeString("c6a5026a")

	tokenInPadded := addressToBytes32(tokenIn)
	tokenOutPadded := addressToBytes32(tokenOut)
	amountInPadded := uint256ToBytes32(amountIn)
	feePadded := uint256ToBytes32(fee)
	sqrtPriceLimitPadded := uint256ToBytes32(big.NewInt(0))

	var data []byte
	data = append(data, selector...)
	data = append(data, tokenInPadded...)
	data = append(data, tokenOutPadded...)
	data = append(data, amountInPadded...)
	data = append(data, feePadded...)
	data = append(data, sqrtPriceLimitPadded...)

	return data
}

func addressToBytes32(addr string) []byte {
	addr = strings.TrimPrefix(strings.ToLower(addr), "0x")
	b, _ := hex.DecodeString(fmt.Sprintf("%064s", addr))
	return b
}

func uint256ToBytes32(v *big.Int) []byte {
	b := v.Bytes()
	padded := make([]byte, 32)
	copy(padded[32-len(b):], b)
	return padded
}

// ExecuteSwap executes a swap via Uniswap V3 on Base.
func ExecuteSwap(accountID, fromSymbol, toSymbol, amount string) (*Trade, error) {
	w := GetWallet(accountID)
	if w == nil {
		return nil, fmt.Errorf("no trading wallet — create one at /trade first")
	}

	// Get quote first
	quote, err := GetQuote(fromSymbol, toSymbol, amount)
	if err != nil {
		return nil, err
	}

	app.Log("trade", "Swap %s → %s for user %s (quoted: %s)", quote.AmountIn, quote.AmountOut, accountID, quote.AmountOut)

	// TODO: Build and sign the actual swap transaction.
	// This requires:
	// 1. Token approval (if ERC20 → approve router to spend)
	// 2. Build exactInputSingle calldata for SwapRouter02
	// 3. Sign transaction with user's private key (secp256k1 + RLP encoding)
	// 4. Submit via eth_sendRawTransaction
	// 5. Wait for confirmation
	//
	// For now, return the quote as a "pending" trade so the UI and agent
	// flow work end-to-end. The actual on-chain execution is the next step.

	t := &Trade{
		ID:        fmt.Sprintf("t_%d", time.Now().UnixNano()),
		Account:   accountID,
		FromToken: fromSymbol,
		ToToken:   toSymbol,
		AmountIn:  quote.AmountIn,
		AmountOut: quote.AmountOut,
		Status:    "quoted",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	saveTrade(accountID, t)

	return t, nil
}
