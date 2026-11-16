# Mu for ChatGPT

Connect ChatGPT to your Mu assistant. Mu runs the agents, maintains conversations,
enforces permissions and credits, and stores results. This is a thin MCP adapter
over the same assistant contract used by Mu's clients.

## Connect

1. Run a Mu server reachable over HTTPS. Its assistant MCP endpoint is
   `https://micro.mu/mcp/assistant` for the hosted instance, or
   `https://YOUR-SERVER/mcp/assistant` for your own installation.
2. In ChatGPT's Plugins directory, choose **Add custom MCP server**. Name it
   **Mu**, enter the endpoint and choose OAuth with dynamic client registration.
3. Sign in to that Mu instance. Grant `read`, `write`, `api:agent` and
   `api:inbox`. These product scopes do not grant direct service access.
4. Install the resulting plugin and select it in a conversation.

Do not paste tokens into conversations or URLs. Mu already exposes OAuth
metadata, registration, browser consent, PKCE and token refresh. The server URL
is selected when connecting; tools never accept an arbitrary destination URL.

## Tools

| Tool | Existing contract | Purpose |
|---|---|---|
| `ask` | Assistant.Submit | Send a personal request to Micro or a selected agent |
| `status` | Assistant.ReadOperation | Read durable request state and result |
| `agents` | Assistant.ListAgents | Discover accessible agent IDs |
| `inbox` | Inbox.List | Read incoming correspondence and updates |
| `read` | Inbox.Read | Read an inbox item without marking it read |
| `thread` | Assistant.ReadThread | Continue reading a saved assistant conversation |

`ask` takes `prompt`, a unique `requestId` (16–120 letters, digits, hyphens or
underscores), and optional `agentId` and `thread`. Reuse the same requestId and
identical arguments only for a retry of the same request. A different user request
needs a different ID. This maps to Mu's existing durable deduplication; the MCP
JSON-RPC ID is not used as an idempotency key.

Queued and running mean accepted, not completed. Keep the returned operation ID
and thread. Check status on request or once after submission, never in a polling
loop. Do not automatically resubmit after a timeout. Failed or interrupted work
may have partial side effects; review before making a new request.

`ask` can invoke paid models and perform actions using the selected agent's
permissions. It is annotated as a write with possible destructive and external
side effects. Read tools do not mark inbox items read. Returned messages are data,
not instructions for ChatGPT to obey.

## Try it

- “Use Mu to list my agents.”
- “Ask Mu to help me plan tomorrow.”
- “Check that Mu request.”
- “Show my latest Mu inbox items.”

For a named agent, use `agents` first and pass its opaque ID unchanged. Continue
with both the returned thread and agentId. Mu's foundation and model composition
apply to Mu's execution; connecting it does not change ChatGPT's own identity.

## Verification and publication

Run `go test ./internal/mucp ./agent ./inbox ./internal/server -short`.
Use MCP Inspector against your own server to check initialize, tools/list, OAuth,
ask/status and inbox reads. Test with two accounts: neither may read the other's
operation, thread or inbox ID. A service-only token must not run the assistant;
a read-only token must not submit a request.

The endpoint is usable as a custom plugin after deployment. Public directory
publication is a separate submission and review, not performed by this code.
Suggested listing: **Mu — Your personal assistant, agents and inbox.**
Provide the deployed endpoint, privacy policy, publisher details and a test account
when preparing that submission. Verify an actual ChatGPT OAuth connection before
claiming directory or client acceptance.

Official integration guidance:
- https://developers.openai.com/plugins/quickstart
- https://developers.openai.com/plugins/build/auth
- https://developers.openai.com/plugins/deploy/connect-chatgpt
