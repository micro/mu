# XMPP Chat Federation

Mu includes an XMPP (Extensible Messaging and Presence Protocol) server that provides federated chat capabilities, similar to how SMTP provides federated email.

## Overview

Just like the mail system uses SMTP for decentralized email, Mu can use XMPP for decentralized chat. This provides:

- **Federation**: Users can communicate across different Mu instances
- **Standard Protocol**: Compatible with existing XMPP clients (Conversations, Gajim, etc.)
- **Autonomy**: No reliance on centralized chat platforms
- **Privacy**: Self-hosted chat infrastructure

## Configuration

The XMPP server is disabled by default. To enable it, set the following environment variables:

```bash
# Enable XMPP server
export XMPP_ENABLED=true

# Set your domain (required for federation)
export XMPP_DOMAIN=chat.yourdomain.com

# Set the port (optional, defaults to 5222)
export XMPP_PORT=5222
```

## DNS Configuration

For federation to work, you'll need to configure DNS SRV records:

```
_xmpp-client._tcp.yourdomain.com. 86400 IN SRV 5 0 5222 chat.yourdomain.com.
_xmpp-server._tcp.yourdomain.com. 86400 IN SRV 5 0 5269 chat.yourdomain.com.
```

## Usage

### Connecting with XMPP Clients

Users can connect to your Mu instance using any XMPP-compatible client:

**Connection Details:**
- **Username**: Your Mu username
- **Domain**: Your XMPP_DOMAIN
- **Port**: 5222 (default)
- **JID Format**: username@yourdomain.com

**Recommended Clients:**
- **Mobile**: Conversations (Android), Siskin (iOS)
- **Desktop**: Gajim (Linux/Windows), Beagle IM (macOS)
- **Web**: Converse.js

### Authentication

The XMPP server integrates with Mu's authentication system. Users authenticate with their Mu credentials using SASL PLAIN authentication.

## Features

### Current Implementation

- **Client-to-Server (C2S)**: Users can connect and send/receive messages
- **Basic Authentication**: SASL PLAIN mechanism
- **Presence**: Online/offline status tracking
- **Resource Binding**: Multiple devices per user
- **Message Routing**: Local message delivery

### Planned Features

- **Server-to-Server (S2S)**: Federation with other XMPP servers
- **Message History**: Persistent chat storage (MAM - Message Archive Management)
- **Multi-User Chat (MUC)**: Group chat rooms
- **File Transfer**: Share files between users
- **End-to-End Encryption**: OMEMO support
- **Push Notifications**: Mobile push via XEP-0357

## Architecture

The XMPP server follows the same pattern as the SMTP server:

```
chat/
├── chat.go          # Web-based chat interface
├── xmpp.go          # XMPP server implementation
└── prompts.json     # Chat prompts
```

Like SMTP, the XMPP server:
- Runs in a separate goroutine
- Listens on a dedicated port (5222 by default)
- Integrates with Mu's authentication system
- Provides autonomy and federation

## Status Monitoring

The XMPP server status is visible on the `/status` page:

```json
{
  "services": [
    {
      "name": "XMPP Server",
      "status": true,
      "details": "chat.yourdomain.com:5222 (3 sessions)"
    }
  ]
}
```

## Security Considerations

### Current Implementation

- SASL PLAIN authentication (credentials sent in plaintext)
- No TLS encryption yet

### Production Recommendations

1. **Use TLS**: Add STARTTLS support for encrypted connections
2. **Strong Authentication**: Implement SCRAM-SHA-256 in addition to PLAIN
3. **Rate Limiting**: Implement connection and message rate limits
4. **Spam Prevention**: Add anti-spam measures
5. **Monitoring**: Track failed authentication attempts

## Comparison with SMTP

| Feature | SMTP (Mail) | XMPP (Chat) |
|---------|-------------|-------------|
| Protocol | RFC 5321 | RFC 6120 |
| Port | 2525/587 | 5222 |
| Federation | Yes | Yes |
| Real-time | No | Yes |
| Offline Delivery | Yes | Planned |
| Encryption | DKIM/SPF | TLS/OMEMO |

## Example Use Cases

### 1. Self-Hosted Chat
Run your own chat server without depending on Discord, Slack, or WhatsApp.

### 2. Federated Communities
Connect multiple Mu instances for a distributed community.

### 3. Privacy-Focused Messaging
Chat with end-to-end encryption on your own infrastructure.

### 4. Integration with Existing Tools
Use existing XMPP clients and bots with your Mu instance.

## Troubleshooting

### Server Won't Start

Check logs for errors:
```bash
# Look for XMPP server logs
mu --serve | grep xmpp
```

Common issues:
- Port 5222 already in use
- Incorrect XMPP_DOMAIN configuration
- Missing permissions to bind to port

### Can't Connect from Client

Verify configuration:
1. Check XMPP_ENABLED is set to true
2. Verify XMPP_DOMAIN matches your setup
3. Ensure port 5222 is accessible (firewall rules)
4. Check DNS SRV records are configured

### Messages Not Delivering

- Ensure both users are connected
- Check server logs for routing errors
- Verify JID format (user@domain/resource)

## Future Development

The XMPP implementation is currently minimal, providing basic chat functionality. Future enhancements include:

1. **Complete S2S Implementation**: Full federation with other XMPP servers
2. **XEP Compliance**: Implement more XMPP Extension Protocols
3. **Message Archive Management**: Persistent message history
4. **Group Chat**: Multi-user chat rooms (MUC)
5. **Modern Features**: Reactions, typing indicators, read receipts
6. **Mobile Support**: Push notifications for offline users

## References

- [RFC 6120](https://tools.ietf.org/html/rfc6120) - XMPP Core
- [RFC 6121](https://tools.ietf.org/html/rfc6121) - XMPP Instant Messaging
- [XMPP Standards Foundation](https://xmpp.org/)
- [XEPs](https://xmpp.org/extensions/) - XMPP Extension Protocols

## Contributing

The XMPP implementation is a work in progress. Contributions welcome for:
- S2S federation
- Additional XEP implementations
- TLS/STARTTLS support
- Enhanced authentication mechanisms
- Testing and documentation

See [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.
