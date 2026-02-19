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

Replace `YOUR_TOKEN` with a Personal Access Token created at `/token`.

For self-hosted instances, replace `mu.xyz` with your domain.

## Available Tools

| Tool | Description |
|------|-------------|
| `chat` | Chat with AI assistant |
| `news` | Read the latest news feed |
| `news_search` | Search for news articles |
| `blog_list` | Get all blog posts |
| `blog_read` | Read a specific blog post |
| `blog_create` | Create a new blog post |
| `blog_update` | Update a blog post |
| `blog_delete` | Delete a blog post |
| `video` | Get the latest videos |
| `video_search` | Search for videos |
| `mail_read` | Read mail inbox |
| `mail_send` | Send a mail message |
| `search` | Search across all content |
| `wallet_balance` | Get wallet credit balance |

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

## Authentication

The MCP endpoint itself is public — any client can connect and discover tools. Authentication is required for tool calls that access protected services. Include your PAT token via the `Authorization` header:

```
Authorization: Bearer YOUR_TOKEN
```

Tools that require authentication (like `mail_read`, `blog_create`) will return errors if no valid token is provided. Public tools (like `news`, `blog_list`) work without authentication.

## Self-Hosting

When running your own Mu instance, the MCP server is available automatically at `/api/mcp` with no additional configuration required.
