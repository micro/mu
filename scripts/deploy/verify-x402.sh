#!/bin/sh
# Verify x402 is wired on the MCP endpoint of a Mu instance.
# Usage: sh deploy/verify-x402.sh [https://host]   (default https://m3o.com)
H="${1:-https://m3o.com}"
echo "Probing $H/mcp"

echo "\n[1] tools/list (free) — MCP up?"
curl -sS -m 25 -X POST "$H/mcp" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | grep -q '"tools"' \
  && echo "  OK: MCP responding" || echo "  FAIL: no tool list"

echo "\n[2] paid tool, no payment — expect HTTP 402 with a conformant accepts body"
resp=$(curl -sS -m 40 -D - -o /tmp/x402body -X POST "$H/mcp" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"news_search","arguments":{"query":"btc"}}}')
echo "$resp" | grep -iq '^HTTP.*402' && echo "  OK: 402 challenge" || echo "  MISSING: no 402 status"
grep -q '"accepts"' /tmp/x402body && echo "  OK: accepts[] body (x402 spec)" || echo "  MISSING: no accepts[] in body"
grep -q '"maxAmountRequired"' /tmp/x402body && echo "  OK: atomic maxAmountRequired" || echo "  MISSING: no maxAmountRequired"
grep -o '0x[0-9a-fA-F]\{40\}' /tmp/x402body | head -1 | sed 's/^/  pay-to advertised: /' || echo "  MISSING: no pay-to address advertised"

echo "\n[3] paid tool WITH X-PAYMENT — expect settle attempt, not 'authentication required'"
curl -sS -m 40 -X POST "$H/mcp" -H 'Content-Type: application/json' -H 'X-PAYMENT: test' \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"news_search","arguments":{"query":"btc"}}}' \
  | grep -q 'authentication required' && echo "  FAIL: X-PAYMENT ignored (payment path unreachable)" || echo "  OK: payment header taken into account"
