# MCP Server

Mu includes an [MCP](https://modelcontextprotocol.io) (Model Context Protocol) server that allows AI assistants and tools to interact with Mu services programmatically.

## Overview

The MCP server exposes Mu services (blog, chat, news, video, mail, search) as tools that any MCP-compatible client can use. It implements the [MCP specification](https://spec.modelcontextprotocol.io) using the Streamable HTTP transport at a single endpoint.

**Endpoint:** `POST /api/mcp`

## Configuration

Add Mu as an MCP server in your client configuration:

```json
{
  "mcpServers": {
    "mu": {
      "url": "https://mu.xyz/api/mcp",
      "headers": {
        "Authorization": "Bearer YOUR_TOKEN"
      }
    }
  }
}
```

Replace `YOUR_TOKEN` with a session token from the `login` tool or a Personal Access Token created at `/token`.

For self-hosted instances, replace `mu.xyz` with your domain.

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
| `blog_create` | Create a new blog post | Free |
| `blog_update` | Update a blog post | Free |
| `blog_delete` | Delete a blog post | Free |
| `video` | Get the latest videos | Free |
| `video_search` | Search for videos | 2 credits |
| `mail_read` | Read mail inbox | Free |
| `mail_send` | Send a mail message | 4 credits |
| `search` | Search across all content | Free |
| `wallet_balance` | Get wallet credit balance | Free |

### Credits

MCP tools use the same wallet credit system as the REST API:
- **1 credit = 1 penny (£0.01)**
- **10 free queries per day** (covers chat, news search, video search)
- Metered tools will return an error if you have insufficient credits
- Admin accounts have unlimited access

## Authentication

AI agents can authenticate using the `login` or `signup` tools:

### Sign Up

```bash
curl -X POST https://mu.xyz/api/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"signup","arguments":{"id":"myagent","secret":"password123","name":"My Agent"}}}'
```

### Log In

```bash
curl -X POST https://mu.xyz/api/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"login","arguments":{"id":"myagent","secret":"password123"}}}'
```

Both return a session token. Use it in subsequent requests:

```
Authorization: Bearer SESSION_TOKEN
```

Tools that require authentication (like `mail_read`, `blog_create`) will return errors if no valid token is provided. Public tools (like `news`, `blog_list`) work without authentication.

## Protocol

The MCP server uses the Streamable HTTP transport. Clients send JSON-RPC 2.0 requests via POST:

### Initialize

```bash
curl -X POST https://mu.xyz/api/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","clientInfo":{"name":"example","version":"1.0"},"capabilities":{}}}'
```

### List Tools

```bash
curl -X POST https://mu.xyz/api/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
```

### Call a Tool

```bash
curl -X POST https://mu.xyz/api/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"news","arguments":{}}}'
```

## Self-Hosting

When running your own Mu instance, the MCP server is available automatically at `/api/mcp` with no additional configuration required.
