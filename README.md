# Mu

The runtime for **Micro, a personal assistant**.

Ask on the web, send an email, or message it from your phone. Mu runs the
assistant, its tools, and your saved conversations in one Go binary that you
can host yourself. Try the hosted instance at [micro.mu](https://micro.mu).

## A small interface

The web starts with one input. Ask a question or give an instruction; the answer
appears below it. The prompt moves up on the first request and stays there while
you read and continue the conversation.

- **Inbox** keeps saved conversations and incoming messages together. Return to
  a web conversation and continue where you left off.
- **Account** holds your profile, connections, client credentials, and billing.
- **Admin**, visible to administrators, holds users, settings, logs, and server
  controls.

One shared stylesheet (`/mu.css`) and browser script (`/mu.js`) serve the pages.
HTML is rendered in Go. There is no frontend build step. The site includes a
manifest and service worker so it can be installed as a PWA.

## Reach the same assistant in different ways

Open **Contact** in the footer, or **Account → Reach Micro**, for the addresses
and numbers configured on your instance. Add Micro to your phone's contacts
from that page.

| Channel | How it fits |
|---|---|
| Web | Start a conversation or reopen one from Inbox. |
| Email | Send or forward a message to `agent@your-domain` from your verified email address. Include what you want the assistant to do. |
| SMS | Verify your phone number in Account, then text the configured number. |
| WhatsApp | Use your verified number to message the configured WhatsApp sender. |
| XMPP | Connect with your account and a Chat token from Client access, then message the agent. |

Email, SMS, WhatsApp, and XMPP require the corresponding server configuration.
SMS and WhatsApp use Twilio; they are not enabled merely by installing Mu.
Contact lists the configured ways to reach the assistant. Incoming mail must
pass the sender checks; account identity comes from verified addresses and
numbers, not from whatever a message claims.

Conversations from the different channels appear in one inbox, but are still
separate threads. Switching from a web conversation to a fresh SMS does not
automatically continue that exact thread. Replies retain their channel context.
Forwarding mail to your own mailbox stores it; addressing an agent asks it to
act. WhatsApp replies are subject to the provider's messaging window.

## Agents and services

**Micro** is the default agent. Agents have instructions and a permitted set of
tools. Services provide those tools: mail, files, calendar, search, weather,
notes, shell, and more. You ask for an outcome; the agent chooses the tools.

Optional Google connections provide access to Gmail, Calendar, Contacts, and
Drive with your consent. User-created agents can have different instructions
and access; mail to `you+research@your-domain` addresses your Research agent.

An explicitly requested job can run in the background and return its result to
the originating conversation. Execution lives under `agent/work`; task records
live in `service/tasks`. The personal daily brief remains available. Other
unsolicited model-generated feeds are disabled.

## Use your existing clients

**Account → Client access** shows connection details and creates tokens with an
explicit choice of Mail, Chat, or both.

- **IMAP** reads the inbox; **SMTP submission** sends mail using a Mail token.
- **XMPP** uses a Chat token.
- **SFTP** transfers stored files using a registered SSH key.
- **SSH** opens an interactive terminal in your account's sandbox using that key.
  It does not grant access to the host machine. Remote exec and port forwarding
  are not supported.

SSH/SFTP must be enabled by the operator. Mail and XMPP public TLS endpoints
require the proxy setup described in the installation guide. The page reports
configured settings; it cannot verify that external DNS, ports, or proxies work.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/micro/mu/main/install.sh | sh
mu setup
mu --serve
```

Open **http://localhost:8080**. Initial administrator setup depends on the
instance's bootstrap configuration; see the installation guide.

Configure an AI provider with `mu setup`. Supported settings include
`ANTHROPIC_API_KEY`, `ATLASCLOUD_API_KEY`, `GEMINI_API_KEY`,
`OPENROUTER_API_KEY`, or an OpenAI-compatible `OPENAI_BASE_URL`.
Individual tools may need provider keys, such as `BRAVE_API_KEY` for web search
or `YOUTUBE_API_KEY` for video. Google sign-in and payments are optional.

From source:

```bash
git clone https://github.com/micro/mu
cd mu
go install
mu setup
mu --serve
```

Or run `docker compose up` from the checkout. See the
[installation guide](docs/INSTALL.md) for domains, TLS, mail, messaging,
sandbox configuration, and deployment.

## CLI and programmatic access

The binary also acts as a client. It defaults to the hosted instance; set
`MU_URL` or use `mu login https://your.host` for your own server.

```bash
mu ask "What needs my attention?"
mu inbox list
mu help
```

CLI and API operations require a credential with the appropriate API or service
permissions. The current browser token form issues Mail/Chat protocol tokens;
those do **not** grant CLI, agent API, or MCP access. Existing API and
service-scoped credentials remain supported.

The JSON API at `/api/v1` and MCP protocol at `/mcp` retain Agent, Work, and Inbox
operations. Service-scoped credentials select service operations instead. The
old browser API and MCP documentation pages have been removed.

```bash
curl "$MU_URL/api/v1/agent/ask" \
  -H "Authorization: Bearer $MU_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"What needs my attention?"}'
```

The response includes `data.text` and `data.thread`; send the thread identifier
back to continue. Requests and credentials belong in request bodies and headers.
A separately configured x402 host provides paid service calls.

## Development and releases

```bash
go build ./...
```

Pages, styles, scripts, and bundled service data are embedded in the binary;
changes require rebuilding. Server settings are available in **Admin → Settings**.

`VERSION` names the release. Updating it on `main` runs the release workflow,
which builds Linux and macOS binaries, creates the version tag, and publishes
the release. A version-tag push or manual workflow dispatch is also supported.

## License

AGPL 3.0
