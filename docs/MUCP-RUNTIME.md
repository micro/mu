<!-- SPDX-License-Identifier: Apache-2.0 -->

# Mu runtime contracts over MUCP

Status: implementation profile for Mu's current `mu.assistant.v1` and direct
service contracts. This is separate from the [MUCP core](MUCP.md) and
[HTTPS binding](MUCP-HTTP.md). Implementing MUCP does not require implementing
Mu, an assistant, a model provider, billing, Home or Inbox.

The schemas in [proto](https://github.com/micro/mu/tree/main/proto) are normative
for field names, types, defaults and enum values at the same repository revision
as this document. Use a pinned revision of the specification and schemas together.
Generated types do not define authorization or operation semantics by themselves.
The sections below define those semantics and identify Mu-specific integration.

## Layers

MUCP carries requests and responses without prescribing a runtime's architecture.
Mu's service contracts describe domain capabilities, including Chat, Groups and
Contacts. Home, Inbox, Agent and Services compose those capabilities into the
assistant experience. Inbox provides the local view of messages and notifications;
the underlying services retain their records. Agent owns prompt execution and
responses through its existing runtime loop.

A protobuf `service` declaration names an RPC interface; it does not move Agent
into the domain-service layer. Protocol adapters MUST preserve the authorization
and ownership boundaries of the operations they expose. They MUST NOT create a
second execution path or allow domain services to invoke assistant execution.

## Agent callers

An external agent is a protocol client. It authenticates with a credential granted
by the runtime, discovers accessible agents, selects an opaque agent ID and uses
Assistant.Submit/ReadOperation to delegate work. It need not be an Android, web
or desktop application. It receives only the authority of its credential; an
agent name, prompt or routing header never establishes identity or delegation.

This profile supports a caller accessing an assistant on one runtime. It does not
define federated agent identity, cross-runtime trust, onward delegation, or
permission to forward the caller's credentials. Those need separate contracts.

## Discovery profile

Mu advertises service `assistant`, contract `mu.assistant.v1` and its registered
assistant/composition operations in the public discovery document. Its features
are `threadRecovery`, `durableSubmission`, `streaming`, `cancellation` and
`automaticRetry`; only the first two are currently true. Its limits are
`requestBytes: 65536`, `waitSeconds: 120`, `pageSize: 100`.

This is not a public inventory of every direct service. Authenticated
Services.List/Describe return supported service contracts and operations. Current
direct services are Contacts, Chat and Groups. A schema alone is not discovery.

## Accounts and sign-in

**Hosted web app:** when `/app/` and the selected runtime have the same origin,
the app uses the existing HttpOnly session cookie. `/session` checks sign-in and
refreshes the readable CSRF cookie. Every cookie-authenticated write MUST include
`X-CSRF-Token`; the runtime verifies it against the authenticated session. Logged-out
users go through the normal login/signup flow and return to `/app/`.

**Independent web, Android and desktop apps:** use browser OAuth authorization
code flow with S256 PKCE, state verification and `read write api:agent api:home api:inbox api:services` scopes.
Native apps use the system browser. The desktop callback uses a loopback listener,
a fresh state and a ten-minute validity window. Authorization codes are consumed
once; the PKCE verifier is not sent in the authorization URL. Client tokens MUST
only be sent to the selected runtime. Switching servers clears the app's session.

OAuth metadata at `/.well-known/oauth-authorization-server` and
`/.well-known/oauth-protected-resource` advertises the scopes available for
consent: read/write, assistant capabilities and registered `service:<name>`
scopes. Clients request only the capabilities they need. Advertised scopes are
not grants; consent and runtime authorization remain required.

The token response includes a 24-hour access token and a rotating refresh token.
Clients renew before expiry with a form POST to `/oauth/token` containing
`grant_type=refresh_token`, `client_id` and `refresh_token`, then securely replace
both credentials. Refresh tokens expire after 90 days without renewal; active
connections remain signed in. Renewal preserves approved scopes and the connection
ID. Deleting the connection/account, disabling the account, or removing the OAuth
client prevents renewal. Reusing a spent refresh token revokes that connection.
Clients serialize renewal and do not replay assistant actions to recover authentication.
Android stores credentials with the Keystore; desktop uses OS-encrypted storage;
independent browser sessions use session storage. Network failures retain local
credentials. Existing installations without a refresh token need one new sign-in
with the updated client; expired access tokens cannot be upgraded.

Native/programmatic calls send `Authorization: Bearer <token>`. Browser bearer
calls omit cookies. An explicit credential takes precedence over any cookie and
an invalid token MUST NOT fall back to cookie identity. Reads require read access;
Writes require write access. Product capabilities are explicit: `api:agent`
for Assistant operations, `api:home` for the composed Home overview (including its
recent inbox preview), `api:inbox` for Inbox, and `api:services` for catalogue
metadata. Existing tokens need renewed consent for newly requested capabilities.
Service-only tokens cannot invoke product operations; product-only tokens cannot
call raw services. Direct Contacts calls require `service:contacts` plus read/write
permissions as appropriate. Discovery never grants execution rights. All account, verification, quota and tool checks
remain enforced at the runtime boundary.

Cross-origin CORS is non-credentialed and limited to MUCP plus `/oauth/register`
and `/oauth/token`. It never grants account access or exposes dashboard pages.
Session cookies and CSRF tokens MUST NOT be sent to another origin. Models and
provider API keys remain on the runtime, never in clients.

## Assistant operations

| Endpoint | Request | Result |
| --- | --- | --- |
| `Assistant.ListAgents` | Empty object | Accessible agents and public descriptions; no private prompts |
| `Assistant.ListThreads` | Optional agent filter, page size and page token | Authorized personal assistant threads and next-page token |
| `Assistant.ReadThread` | Thread ID and optional pagination | Thread metadata, ordered messages, next-page token and `pending` |
| `Home.Overview` | Empty object | Accessible agents, recent inbox, pinned services, cached events and current public brief |
| `Inbox.List` | Pagination and unread filter | Owned inbox items across channels; held items excluded |
| `Inbox.Read` | Item ID and pagination | Safe messages without changing read state |
| `Inbox.MarkRead` | Item ID and unread boolean | Persisted read state |
| `Services.List` | Pagination | Service descriptions and supported direct contracts |
| `Services.Describe` | Service name | Description and implemented direct operations |
| `Assistant.Submit` | Stable message ID and PERSONAL Ask request | Durable operation receipt |
| `Assistant.ReadOperation` | Operation ID | Owned operation state and terminal result |
| `Assistant.Ask` | Agent ID, thread ID or empty string, prompt, access mode and voice preference | Thread ID, responding agent, Markdown text and typed content |

The protobuf schema defines all fields and enum values. Use `thread`, not a second
conversation identifier. The runtime derives ownership from credentials, checks
access on each call and excludes service-owned mail/chat records from assistant
thread history. Inaccessible and nonexistent threads both return 404.

`ACCESS_MODE_PERSONAL` creates or continues a durable personal assistant thread.
`ACCESS_MODE_PUBLIC` is still authenticated and metered, has restricted tools and
is stateless: it cannot continue a thread. Unspecified or unknown modes fail.
The `voice` field describes the interaction; it does not grant background listening
or require an audio transport. Clients own dictation and playback.


### Durable personal submission

`Assistant.Submit` returns an operation after the request and queue receipt have
been persisted. `messageId` is 16–120 ASCII alphanumeric, hyphen or underscore
characters, scoped to the caller. It is distinct from the exchange's `Micro-Id`.
Reusing it with the same request returns the same operation; changing the agent,
thread, prompt or voice preference returns a conflict. Leading/trailing prompt
whitespace is normalized. Omitted thread and the returned thread identify the same
submission. Completed receipts are retained for at least seven days from creation;
clients MUST NOT replay an ambiguous submission after that window. Account/thread
deletion or revoked agent access revokes recovery. This is deduplicated submission,
not a guarantee that an external tool produces its side effects exactly once.

ReadOperation returns queued, running, succeeded, failed or interrupted. A success
contains the same answer/content as Ask. Resume by operation ID; if the acceptance
response was lost, repeat Submit with the original message ID and unchanged body
within the retention window. Never generate a new message ID merely to retry.
A runtime restart preserves queued work and records interrupted in-flight work.
Operations are not universal approval or cancellation handles.

### Direct service contracts

`Micro-Service: chat` supports `Chat.Rooms`, `Chat.Messages`, `Chat.Send`,
`Chat.Source` and `Chat.Sources`. `Micro-Service: groups` supports `Groups.List`,
`Groups.Read`, `Groups.Create` and `Groups.Update`. These require their own
`service:chat` or `service:groups` scope plus read/write permission; assistant
product scopes do not grant them. Both services implement generated protobuf
request/response types and use the same gateway as REST and MCP. Group reads and
sends check current membership, including after invitation acceptance or removal.
Invitation hashes are storage-only and absent from the public contract.

Direct `Chat.Send` is a service/tool post, not an instruction to run an agent.
Interactive group-agent execution remains in the runtime chat ingress, where
mentions, group tool restrictions and group billing apply. Clients must not infer
assistant execution from successful delivery. Other service schemas under
`proto/service` do not imply available MUCP endpoints; consult discovery.

`Micro-Service: contacts` uses `Contacts.List`, `Contacts.Find`, `Contacts.Add`
and `Contacts.Delete`, defined in `proto/service/contacts/v1`. These adapt the
existing service gateway and its account/metering checks. Delete is an explicit
write and remains unavailable as an autonomous destructive model tool. Other
services remain available to agents through their existing tools; their catalogue
entries have no direct MUCP contract until a tested adapter ships.

Home reads local projections and schedules the existing bounded background cache
refresh. It does not wait on remote calendars,
invoke a model, or spend credits to render an overview. `calendarPending` signals
stale/missing calendar cache. Inbox reads do not grant permission to execute agents
on mail/group contexts. Current Inbox message projection exposes text and role;
personal assistant threads additionally expose structured result blocks.

## Completion and recovery

PERSONAL Ask submits to the existing persistent reply queue and waits up to 120
seconds. HTTP 200 returns the completed answer. Once acceptance is known, the
runtime exposes the thread in `Micro-Thread`, including on pending failures.

A 504 with `Micro-Thread` means the HTTP wait ended without a verified completed
response. ReadThread can recover saved messages and reports whether work remains
pending. A disconnected client can resume with its known thread; if the thread ID
was not received, ListThreads lets the user find the new chat. Clients SHOULD use
bounded polling and stop polling when the view is left or the outcome is terminal.

A network failure or generic intermediary 504 does **not** prove acceptance or
rejection. Clients MUST NOT automatically resend Ask. The queue deduplicates some
retained personal submissions, but there is no general replay window or permanent
idempotency guarantee. Retries can repeat paid work and external side effects.

Closing a socket, changing agents, interrupting speech or stopping local playback
MUST NOT be described as cancelling server work. There is no cancel operation.
Restarted in-flight work records interruption instead of blindly replaying tools.
PUBLIC requests have no durable-thread recovery guarantee.

Pagination is live, not a snapshot. Tokens are signed and scoped to account,
operation and filter; they expire after fifteen minutes or a runtime restart.
Clients restart pagination after expiration and deduplicate by record ID.
ReadThread is oldest-first. Maximum page size is 100.

## Content and client safety

A reply has Markdown text and typed content blocks: sources, images, audio, video,
routes and resource references. Unknown fields are additive; unknown blocks use
fallback text. Clients MUST treat content as data, reject unsafe URL schemes and
avoid executing model-authored HTML, scripts or commands. Media references do not
promise background playback or override provider restrictions.

Rendered links are not tool permissions. Server-held confirmations and executable
actions require a future contract; clients MUST NOT invent them from model text.
Do not put prompts, credentials or private search text in URLs or access logs.
Opaque thread IDs may appear in navigation; protocol payloads carry them in bodies.

## Operation state and interoperability limits

The wire values are the OPERATION_STATE_* enums in assistant.proto. Queued and
running are nonterminal. Succeeded carries a result; failed and interrupted carry
an error description. Clients MUST accept a terminal state on the first read:
polling need not observe every intermediate state. An interrupted result does
not prove that external effects were rolled back. Do not automatically resubmit.

The current profile does not specify a portable scheduler or exactly-once tool
execution. Stronger terminal immutability, ordering between simultaneous requests
on one thread, and the precise race between deletion and receipt recovery need
further specification and implementation tests before being promised. Mu currently
serializes submission acceptance in one process. That lock is not a protocol
requirement for other implementations.

Mu's OAuth URLs, token lifetimes, local credential stores and signed pagination
implementation above describe its deployment. Independently implemented servers
need compatible externally observable authorization and pagination behaviour,
not Mu's storage layout or platform-specific credential code. A general OAuth
bootstrap/discovery profile remains work ahead; the core does not pretend that
knowing a runtime origin is sufficient to acquire credentials automatically.

See the [conformance plan](MUCP-CONFORMANCE.md) for tested claims and remaining
work. This profile is Apache-2.0 licensed; see [licence](../proto/LICENSE).
