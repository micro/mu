# Mu

The runtime for **Micro, a personal assistant**.

## Overview

Ask on the web, send an email, or message it from your phone. Mu runs the
assistant, its tools, and your saved conversations in one Go binary that you
can host yourself. Try the hosted instance at [micro.mu](https://micro.mu).

## Features

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

## Clients

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

## Agents

**Micro** is the default agent. Agents have instructions and a permitted set of
tools. Services provide those tools: mail, files, calendar, search, weather,
notes, shell, and more. You ask for an outcome; the agent chooses the tools.

## Google

Optional Google connections provide access to Gmail, Calendar, Contacts, and
Drive with your consent. User-created agents can have different instructions
and access; mail to `you+research@your-domain` addresses your Research agent.

Google reads use the signed-in account's connection and granted scope. Gmail
searches default to the last 30 days unless an explicit date range is supplied.
The assistant carries at most 24 recent messages within a 16,000-character
history budget. Older conversations and saved notes are read through permitted
tools when needed, rather than automatically added to every question. Disconnecting
Google stops new reads; it does not erase answers already saved in your Inbox.

## Work

An explicitly requested job can run in the background and return its result to
the originating conversation. Execution lives under `work`; task records
live in `service/tasks`. The personal daily brief remains available. Other
unsolicited model-generated feeds are disabled.

## Protocols

**Account → Client access** shows connection details and creates tokens with an
explicit choice of Mail, Chat, or both.

- **IMAP** reads the inbox; **SMTP submission** sends mail using a Mail token.
- **XMPP** uses a Chat token.
- **SFTP** transfers stored files using a registered SSH key.
- **SSH** opens an interactive terminal in your account's sandbox using that key.
  It does not grant access to the host machine. Remote exec and port forwarding
  are not supported.

SSH/SFTP must be enabled by the operator. Legacy `SHELL_SHARED` execution is
blocked; existing shared workspaces need migration to per-account containers. Mail and XMPP public TLS endpoints
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

Configure an AI provider with `mu setup`, or use the settings below.
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

## Models and providers

Mu supports these model providers. Available models depend on the provider
credentials and model IDs configured on your instance; the agent requires a
model with tool-calling support.

| Provider | Models | Configuration |
|---|---|---|
| Anthropic | Claude Opus, Sonnet and Haiku | `AI_PROVIDER=anthropic`, `ANTHROPIC_API_KEY`; optional `ANTHROPIC_MODEL` |
| Atlas Cloud | DeepSeek, Qwen and GLM | `AI_PROVIDER=atlascloud`, `ATLASCLOUD_API_KEY`; optional `ATLAS_MODEL` |
| Google | Gemini | `AI_PROVIDER=gemini`, `GEMINI_API_KEY`; optional `GEMINI_MODEL` |
| OpenRouter | Models available through its catalogue, including GPT, Claude, Gemini and GLM | `AI_PROVIDER=openrouter`, `OPENROUTER_API_KEY`; optional `OPENROUTER_MODEL` |
| OpenAI-compatible endpoint | Models served by your endpoint, including Ollama, vLLM or llama.cpp | `AI_PROVIDER=local`, `OPENAI_BASE_URL`, `OPENAI_MODEL`; `OPENAI_API_KEY` if required |
| Codex admin preview | GPT-6-Astra (`gpt-6-astra`) through a ChatGPT login | Separate host setup and per-account opt-in below; do not set `AI_PROVIDER=codex` |

Set `AI_PROVIDER` explicitly when keeping credentials for multiple providers.
`AGENT_MODEL` can override the agent's model; explicitly named models can also
select their provider. Settings come from environment variables first, then
stored configuration in **Admin → Settings**. For example, a local endpoint:

```sh
export AI_PROVIDER=local
export OPENAI_BASE_URL=http://localhost:11434/v1
export OPENAI_MODEL=your-installed-model
mu --serve
```

Use the model ID actually served by your endpoint. See the
[AI configuration reference](docs/INSTALL.md#ai-provider) for provider settings.

### Configure Codex

Codex is an opt-in preview for an administrator's direct conversations, using
the existing Micro agent. Other accounts and scheduled/background work keep
the site provider. The preview is currently pinned to GPT-6-Astra; it does not
expose every model listed by `mu codex status`.

1. On the server, install Bubblewrap and the standalone native Codex **0.156.1**
   executable. Linux with permitted unprivileged user namespaces is required.
   On Debian/Ubuntu, install Bubblewrap with `sudo apt install bubblewrap`.
2. As the OS user running Micro, use the same Mu data directory and executable
   configuration as the server:

   ```sh
   # If the native executable is not named codex on PATH:
   export CODEX_BINARY=/absolute/path/to/native/codex
   mu codex login
   mu codex check
   ```

   Set `CODEX_BINARY` in the server environment too if you use it here. An npm
   JavaScript launcher is not the native executable. Login creates a separate
   credential profile; check verifies the sandbox and makes a small, real
   GPT-6-Astra request with a harmless tool, using your ChatGPT allowance.
3. After the check succeeds, sign in as an admin and open **Account → Codex
   preview → Enable for my conversations**. Continue in the normal conversation
   UI. Select **Use the site provider** in the same section to turn it off.

Each request uses fresh sandboxed state and an ephemeral Codex thread. Personal
Codex history, configuration, memories and connectors are excluded; Micro
supplies conversation context and retains its account-scoped tool permissions.
The login still uses the chosen ChatGPT account and its allowance. Networking
remains available; the sandbox is not an outbound-network firewall.

A failed check blocks activation, with no unsandboxed fallback. `mu codex status`
only inspects the existing personal CLI login; it does not configure or validate
this preview. See the [Codex installation guide](docs/INSTALL.md#checking-codex-availability)
for isolation details and host requirements.

## CLI

The binary also acts as a client. It defaults to the hosted instance; set
`MU_URL` or use `mu login https://your.host` for your own server.

```bash
mu ask "What needs my attention?"
mu inbox list
mu help
```

CLI and API operations require a credential with the appropriate API or service
permissions. Client access offers separate Mail/Chat protocol tokens, product
tokens with selected capabilities and optional actions, and tokens restricted to
selected services. Protocol tokens do **not** grant CLI, agent API, or MCP access.
Agent API access can execute your account’s agents with their configured tools;
use service scopes when a client should reach only specific capabilities.

## API

Service operations remain at `/api/v1/<service>/<method>` and `/mcp`, documented
at `/api` and `/tools`. The x402 host exposes the same service contract with
payment handling. Credentials restrict access; they do not switch catalogues.

Product clients use the existing resource URLs: `/agent`, `/agent/<name>`,
`/inbox` and `/work`. Content-Type selects JSON or form input; Accept selects
JSON or HTML output. `/developers` documents the supported requests.
There are no separate product API or MCP endpoints.

```bash
curl "$MU_URL/agent" \
  -H "Authorization: Bearer $MU_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"What needs my attention?"}'
```

The response includes `text` and `thread`; send the thread identifier
back to continue. Requests and credentials belong in request bodies and headers.
A separately configured x402 host provides paid service calls.

## Development

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

App builds are queued and return an ID immediately. Use Apps.BuildStatus with
that ID, Apps.Read with the returned slug, or the build status URL to follow the
result. Reuse `request_key` when retrying a submission; without one, identical
requests by the same account resolve to the same build. Build records and model
output are checkpointed under the data directory. Recovery retains the same ID,
counts interrupted model calls against the three-attempt limit, and never
restarts a completed model call just to retry saving. Generated apps start private.
An interrupted provider request can still incur a charge. This queue assumes one
server process owns the data directory; it is not a distributed worker queue.

Anonymous model prompts are disabled by default. `ALLOW_GUEST_AI=true` explicitly
re-enables them with guest limits: 10 calls per browser, 30 per IP and 100 across
the instance per hour (`GUEST_MAX_PER_CLIENT`, `GUEST_MAX_PER_IP`,
`GUEST_MAX_TOTAL`, `GUEST_WINDOW_MINUTES`). These count admitted guest requests,
not individual model/tool calls, and reset on restart. Public pages remain
readable; these controls do not prevent all scraping. Configure `TRUSTED_PROXY`
correctly so client addresses are resolved at the intended boundary.

### Rate limits

Dynamic HTTP routes are limited before page handling. Public reading stays open;
priced service operations require an authenticated, verified/approved account or
an authenticated x402 wallet using the existing payment gate. Assistant runs have
separate account budgets, including failed attempts. Account and operator billing
exemptions do not bypass these attempt budgets.

| Setting | Default |
| --- | --- |
| `HTTP_MAX_PER_MINUTE` | 300 per IP, including authenticated traffic |
| `HTTP_GUEST_MAX_PER_MINUTE` | 60 per IP |
| `HTTP_GUEST_MAX_PER_HOUR` | 300 per IP |
| `AUTH_MAX_PER_15_MINUTES` | 20 non-GET authentication requests per IP |
| `ASSISTANT_MAX_PER_HOUR` | 60 runs per account |
| `ASSISTANT_MAX_PER_DAY` | 300 runs per account |
| `ASSISTANT_MAX_CONCURRENT` | 2 runs per account |
| `PAID_MAX_PER_HOUR` | 300 priced service attempts per account |
| `PAID_MAX_PER_DAY` | 1,000 priced service attempts per account |

These are fixed windows beginning with the first attempt, not monetary spending
caps. Positive settings override defaults; zero does not disable protection.
Known `/mu.css`, `/mu.js`, manifest, favicon, robots and `/static/` assets bypass
HTTP limits. IPv6 clients share a /64 limit. Configure `TRUSTED_PROXY` for your
actual reverse proxies; forwarded chains are read from the trusted end.

Attempt counters are committed to `abuse.db` before work starts. They survive a
process restart when the same data directory is retained. Expired rows are
pruned; the store caps identities at 20,000 and refuses new ones when full.
Storage failure refuses protected requests. Daily successful-operation counts
are also persisted. This supports one server process per data directory, not
multiple independent replicas. Existing credit billing remains separate.

HTTP limits return 429 and `Retry-After`; protection-storage or admission-queue
failures return 503 and `Retry-After`. Tool protocol errors retain their native
format. `/admin/traffic?window=day` includes a 24-hour view, HTTP status/endpoint
counts and instrumented model call counts. Requests rejected before identity
validation appear under `unattributed` and `http-refused`, not under a guessed
account. New counters start at deployment;
2xx HTTP responses can still contain application or MCP errors. Agent model
counts are reported when the run records its usage; an interrupted run may not
report them. These application limits bound ordinary abuse, not distributed
volumetric attacks or all copying of public content.
