# MCP Server

Mu includes an [MCP](https://modelcontextprotocol.io) (Model Context Protocol) server that allows AI assistants and tools to interact with Mu services programmatically.

The MCP server exposes 30+ tools — news, search, video, weather, places, mail, blog, apps, markets — that any MCP-compatible client can use. It implements the [MCP specification](https://spec.modelcontextprotocol.io) using the Streamable HTTP transport at a single endpoint.

**Endpoint:** `POST /mcp`

## Pay with Crypto (x402)

AI agents can pay per-request with stablecoins via the [x402 protocol](https://x402.org). No account, no API key, no signup. Just call and pay.

**Accepted tokens:** USDC and EURC on Base (configurable via `X402_ASSETS`).

When x402 is enabled on the server (`X402_PAY_TO` is set), any metered tool call without sufficient credits returns `HTTP 402` with payment requirements. The agent pays on-chain, retries, and gets the response.

### Example: x402 Payment Flow

**1. Call a tool without payment:**

```bash
curl -X POST https://mu.xyz/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"web_search","arguments":{"query":"latest AI news"}}}'
```

**2. Server returns 402 with payment requirements:**

```
HTTP/1.1 402 Payment Required
X-PAYMENT-REQUIRED: eyJzY2hlbWUiOiJleGFjdCIsIm5ldHdvcmsi...

{
  "error": "Payment required",
  "x402": [{
    "scheme": "exact",
    "network": "eip155:8453",
    "maxAmountRequired": "$0.05",
    "resource": "/mcp",
    "description": "Access to web_search",
    "payTo": "0x...",
    "asset": "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"
  }],
  "accepts": ["x402"]
}
```

**3. Agent pays on-chain and retries with payment proof:**

```bash
curl -X POST https://mu.xyz/mcp \
  -H "Content-Type: application/json" \
  -H "X-PAYMENT: <base64-encoded-payment-payload>" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"web_search","arguments":{"query":"latest AI news"}}}'
```

**4. Server verifies, settles, and returns the result.**

The `X-PAYMENT-REQUIRED` header contains base64-encoded JSON with all the information a client needs to construct a payment: network, asset, amount, and destination address.

### Pricing

Metered tools are priced at **1 credit = $0.01 USD** via x402:

| Tool | x402 Price |
|------|-----------|
| `news_search` | $0.01 |
| `video_search` | $0.02 |
| `social_search` | $0.01 |
| `chat` | $0.03 |
| `web_search` | $0.05 |
| `web_fetch` | $0.03 |
| `weather_forecast` | $0.01 |
| `places_search` | $0.05 |
| `places_nearby` | $0.02 |
| `mail_send` | $0.04 |
| `apps_build` | $0.03 |
| `apps_run` | $0.03 |

Free tools (news, blog_list, blog_read, video, markets, social, search, quran, hadith, reminder, etc.) don't require payment.

## Account-Based Authentication

For human users or agents that prefer account-based billing, authenticate with a session token or Personal Access Token:

```json
{
  "mcpServers": {
    "mu": {
      "url": "https://mu.xyz/mcp",
      "headers": {
        "Authorization": "Bearer YOUR_TOKEN"
      }
    }
  }
}
```

Replace `YOUR_TOKEN` with a session token from the `login` tool or a Personal Access Token created at `/token`.

### Sign Up

```bash
curl -X POST https://mu.xyz/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"signup","arguments":{"id":"myagent","secret":"password123","name":"My Agent"}}}'
```

### Log In

```bash
curl -X POST https://mu.xyz/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"login","arguments":{"id":"myagent","secret":"password123"}}}'
```

Both return a session token. Use it in subsequent requests:

```
Authorization: Bearer SESSION_TOKEN
```

Account-based users get **20 free credits per day** and can top up with a card via Stripe.

## Available Tools

| Tool | Description | Credit Cost |
|------|-------------|-------------|
| `login` | Log in and get session token | Free |
| `signup` | Create account and get session token | Free |
| `chat` | Chat with AI assistant | 3 credits |
| `news` | Read the latest news feed | Free |
| `news_search` | Search for news articles | 1 credit |
| `blog_list` | Get all blog posts | Free |
| `blog_read` | Read a specific blog post | Free |
| `blog_create` | Create a new blog post | 1 credit |
| `blog_update` | Update a blog post | Free |
| `blog_delete` | Delete a blog post | Free |
| `video` | Get the latest videos | Free |
| `video_search` | Search for videos | 2 credits |
| `social` | Read the social feed | Free |
| `social_search` | Search social posts | 1 credit |
| `places_search` | Search for places by name or category | 5 credits |
| `places_nearby` | Find places of interest near a location | 2 credits |
| `mail_read` | Read mail inbox | Free |
| `mail_send` | Send a mail message | 4 credits |
| `search` | Search across all content | Free |
| `wallet_balance` | Get wallet credit balance | Free |
| `wallet_topup` | Get wallet topup payment methods | Free |
| `markets` | Get live market prices | Free |
| `reminder` | Get the daily Islamic reminder | Free |
| `quran` | Look up a Quran chapter or verse | Free |
| `hadith` | Look up hadith from Sahih Al Bukhari | Free |
| `quran_search` | Semantic search across Quran and Hadith | Free |
| `weather_forecast` | Get the weather forecast for a location | 1 credit |
| `web_search` | Search the web for current information | 5 credits |
| `web_fetch` | Fetch a web page and return cleaned readable content | 3 credits |
| `apps_search` | Search the apps directory | Free |
| `apps_read` | Read details of a specific app | Free |
| `apps_create` | Create a new app | Free |
| `apps_edit` | Edit an existing app | Free |
| `apps_build` | AI-generate an app from a description | 3 credits |
| `apps_run` | Run JavaScript code in a sandbox | 3 credits |

## Protocol

The MCP server uses the Streamable HTTP transport. Clients send JSON-RPC 2.0 requests via POST:

### Initialize

```bash
curl -X POST https://mu.xyz/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","clientInfo":{"name":"example","version":"1.0"},"capabilities":{}}}'
```

### List Tools

```bash
curl -X POST https://mu.xyz/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
```

### Call a Tool

With x402 payment:

```bash
curl -X POST https://mu.xyz/mcp \
  -H "Content-Type: application/json" \
  -H "X-PAYMENT: <payment-payload>" \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"web_search","arguments":{"query":"latest news"}}}'
```

With account token:

```bash
curl -X POST https://mu.xyz/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"news","arguments":{}}}'
```

## Two Ways to Pay

| | x402 (Crypto) | Account (Card) |
|---|---|---|
| Setup required | None | Sign up + top up |
| Auth header | `X-PAYMENT` | `Authorization: Bearer` |
| Payment model | Per request | Pre-paid credits |
| Currency | USDC | GBP |
| Free tier | No | 10 queries/day |
| Best for | Autonomous agents | Human users, MCP clients |

## Self-Hosting

When running your own Mu instance, the MCP server is available automatically at `/mcp` with no additional configuration required.

To enable x402 payments, set `X402_PAY_TO` to your wallet address. USDC and EURC on Base are accepted by default. See [Configuration](ENVIRONMENT_VARIABLES.md) for details.
