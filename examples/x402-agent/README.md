# Paying for a tool call with x402

A complete agent that calls a priced tool on a Mu instance and pays for it in
USDC, with no account, no API key and no signup.

It imports nothing from Mu. The only dependency is the [x402
Foundation](https://github.com/x402-foundation/x402) Go SDK, and everything it
knows about the server it learns from the 402 response — so the same file works
against any conformant x402 server. Point `-server` somewhere else and it pays
them instead.

## Run it

You need a wallet on Base holding a little USDC. No ETH: the payment is a
signature the server settles, so the payer never pays gas.

```bash
export X402_PRIVATE_KEY=0x...          # a funded Base wallet
go run . -tool web_search -arg query=x402
```

```
paying from 0x1234…
settled: 0xabc… on base
{"results":[…]}
```

Other tools take other arguments — `-arg` is repeatable:

```bash
go run . -tool weather_forecast -arg lat=51.5 -arg lon=-0.12
go run . -server http://localhost:8081 -tool web_search -arg query=x402
```

## What happens

1. It POSTs an ordinary MCP `tools/call` to `/mcp` with no credentials.
2. The server answers `402` with an `accepts` list naming the price, the asset
   and the address to pay.
3. The SDK signs an [EIP-3009](https://eips.ethereum.org/EIPS/eip-3009)
   transfer authorisation for exactly that amount.
4. It retries with the payment header, the server settles it through a
   facilitator, and the tool answers.

Steps 2–4 are inside `http.Client.RoundTrip`, which is why the calling code has
no payment logic in it. The signature in step 3 moves nothing on its own — the
server has to present it to be settled — so a wallet can hand one out without
trusting the recipient with anything beyond the amount it names.

You can see step 2 for yourself without a wallet at all:

```bash
curl -s -X POST https://micro.mu/mcp -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call",
       "params":{"name":"web_search","arguments":{"query":"x402"}}}'
```

## Prices

Every priced tool names its own price in the 402, so nothing here is hardcoded.
The full list is at [/pricing](https://micro.mu/pricing); one credit is £0.01
and `web_search` is two of them. Most of the catalogue is free, and a free tool
returns a result with no 402 at all — which is worth trying first, because it
needs no wallet.

## Other languages

The same endpoint works with any x402 client. The Foundation ships
[TypeScript, Python, Java and Go](https://github.com/x402-foundation/x402); the
TypeScript `fetch` wrapper is the shortest of them.
