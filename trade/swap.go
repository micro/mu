package trade

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/data"
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

	from, ok := Tokens[strings.ToUpper(fromSymbol)]
	if !ok {
		return nil, fmt.Errorf("unknown token: %s", fromSymbol)
	}
	to, ok := Tokens[strings.ToUpper(toSymbol)]
	if !ok {
		return nil, fmt.Errorf("unknown token: %s", toSymbol)
	}

	amountIn, err := ParseAmount(amount, from.Decimals)
	if err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}

	// Get quote for minimum output (5% slippage tolerance)
	quote, err := GetQuote(fromSymbol, toSymbol, amount)
	if err != nil {
		return nil, err
	}

	// Parse the quoted output amount for slippage calc
	outParts := strings.Fields(quote.AmountOut)
	quotedOut, _ := ParseAmount(outParts[0], to.Decimals)
	// 5% slippage: amountOutMinimum = quotedOut * 95 / 100
	amountOutMin := new(big.Int).Mul(quotedOut, big.NewInt(95))
	amountOutMin.Div(amountOutMin, big.NewInt(100))

	app.Log("trade", "Executing swap %s %s → %s for user %s", amount, fromSymbol, toSymbol, accountID)

	fromAddr := from.Address
	toAddr := to.Address
	if from.Native {
		fromAddr = Tokens["WETH"].Address
	}
	if to.Native {
		toAddr = Tokens["WETH"].Address
	}

	// Step 1: Approve token if ERC20 (not native ETH)
	if !from.Native {
		if err := ensureApproval(w, from.Address, UniswapRouterAddr, amountIn); err != nil {
			return nil, fmt.Errorf("approval failed: %w", err)
		}
	}

	// Step 2: Build swap calldata
	// exactInputSingle: (address tokenIn, address tokenOut, uint24 fee, address recipient, uint256 amountIn, uint256 amountOutMinimum, uint160 sqrtPriceLimitX96)
	selector, _ := hex.DecodeString("414bf389")
	fee := big.NewInt(3000) // Try 0.3% pool
	if quote.PoolFee == "0.05%" {
		fee = big.NewInt(500)
	}
	deadline := new(big.Int).SetInt64(time.Now().Unix() + 300) // 5 minute deadline

	var callData []byte
	callData = append(callData, selector...)
	callData = append(callData, addressToBytes32(fromAddr)...)
	callData = append(callData, addressToBytes32(toAddr)...)
	callData = append(callData, uint256ToBytes32(fee)...)
	callData = append(callData, addressToBytes32(w.Address)...) // recipient
	callData = append(callData, uint256ToBytes32(deadline)...)
	callData = append(callData, uint256ToBytes32(amountIn)...)
	callData = append(callData, uint256ToBytes32(amountOutMin)...)
	callData = append(callData, uint256ToBytes32(big.NewInt(0))...) // sqrtPriceLimitX96

	// Step 3: Build and sign transaction
	txValue := big.NewInt(0)
	if from.Native {
		txValue = amountIn
	}

	nonce, err := getNonce(w.Address)
	if err != nil {
		return nil, fmt.Errorf("get nonce: %w", err)
	}

	maxPriorityFee, maxFee, err := getGasFees()
	if err != nil {
		return nil, fmt.Errorf("get gas fees: %w", err)
	}

	gasLimit, err := estimateGas(w.Address, UniswapRouterAddr, txValue, callData)
	if err != nil {
		return nil, fmt.Errorf("estimate gas: %w", err)
	}

	tx := &Transaction{
		ChainID:              big.NewInt(BaseChainID),
		Nonce:                nonce,
		MaxPriorityFeePerGas: maxPriorityFee,
		MaxFeePerGas:         maxFee,
		GasLimit:             gasLimit,
		To:                   UniswapRouterAddr,
		Value:                txValue,
		Data:                 callData,
	}

	signedTx, err := SignTransaction(tx, w.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("sign transaction: %w", err)
	}

	// Step 4: Submit
	txHash, err := sendRawTransaction(signedTx)
	if err != nil {
		return nil, fmt.Errorf("broadcast failed: %w", err)
	}

	t := &Trade{
		ID:        fmt.Sprintf("t_%d", time.Now().UnixNano()),
		Account:   accountID,
		FromToken: fromSymbol,
		ToToken:   toSymbol,
		AmountIn:  amount + " " + fromSymbol,
		AmountOut: quote.AmountOut,
		TxHash:    txHash,
		Status:    "pending",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	saveTrade(accountID, t)

	// Step 5: Wait for confirmation in background
	go func() {
		gasUsed, err := waitForReceipt(txHash)
		walletMu.Lock()
		defer walletMu.Unlock()
		for _, tr := range trades[accountID] {
			if tr.ID == t.ID {
				if err != nil {
					tr.Status = "failed"
					app.Log("trade", "Swap %s failed: %v", txHash, err)
				} else {
					tr.Status = "confirmed"
					tr.GasUsed = gasUsed
					app.Log("trade", "Swap %s confirmed (gas: %s)", txHash, gasUsed)
				}
				break
			}
		}
		data.SaveJSON("trade_history.json", trades)
	}()

	return t, nil
}

// ensureApproval checks the ERC20 allowance and approves if needed.
func ensureApproval(w *Wallet, tokenAddr, spender string, amount *big.Int) error {
	// Check current allowance: allowance(owner, spender)
	ownerPadded := addressToBytes32(w.Address)
	spenderPadded := addressToBytes32(spender)
	checkData, _ := hex.DecodeString("dd62ed3e") // allowance selector
	checkData = append(checkData, ownerPadded...)
	checkData = append(checkData, spenderPadded...)

	result, err := ethCall(tokenAddr, checkData)
	if err != nil {
		return fmt.Errorf("check allowance: %w", err)
	}

	current := hexToBigInt(strings.Trim(string(result), `"`))
	if current.Cmp(amount) >= 0 {
		return nil // already approved
	}

	// Approve max uint256
	approveData, _ := hex.DecodeString("095ea7b3") // approve selector
	maxUint256, _ := new(big.Int).SetString("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 16)
	approveData = append(approveData, spenderPadded...)
	approveData = append(approveData, uint256ToBytes32(maxUint256)...)

	nonce, err := getNonce(w.Address)
	if err != nil {
		return fmt.Errorf("get nonce: %w", err)
	}

	maxPriorityFee, maxFee, err := getGasFees()
	if err != nil {
		return fmt.Errorf("get gas fees: %w", err)
	}

	gasLimit, err := estimateGas(w.Address, tokenAddr, nil, approveData)
	if err != nil {
		gasLimit = 60000 // fallback for approve
	}

	tx := &Transaction{
		ChainID:              big.NewInt(BaseChainID),
		Nonce:                nonce,
		MaxPriorityFeePerGas: maxPriorityFee,
		MaxFeePerGas:         maxFee,
		GasLimit:             gasLimit,
		To:                   tokenAddr,
		Value:                big.NewInt(0),
		Data:                 approveData,
	}

	signedTx, err := SignTransaction(tx, w.PrivateKey)
	if err != nil {
		return fmt.Errorf("sign approve: %w", err)
	}

	txHash, err := sendRawTransaction(signedTx)
	if err != nil {
		return fmt.Errorf("send approve: %w", err)
	}

	app.Log("trade", "Approval tx sent: %s", txHash)

	// Wait for approval to be mined
	_, err = waitForReceipt(txHash)
	if err != nil {
		return fmt.Errorf("approval tx failed: %w", err)
	}

	return nil
}
