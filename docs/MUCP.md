# Micro Communication Protocol (MUCP)

Status: initial HTTPS/JSON binding implemented at `/`. Android uses generated
Java types; the standalone web client sends ordinary JSON. Mu continues to use go-micro
internally. Streaming, cancellation and additional transports remain future work.

## Purpose

MUCP carries service requests, responses, streams and events independently of
the transport and body encoding. Reuse go-micro's implementation rather than
introducing another RPC stack. Mu defines the assistant contract carried by it;
clients present its results using their own platform controls.

A conventional HTTP API remains sufficient for individual requests. The value
here is a shared, documented contract for conversations, structured results,
completion, cancellation and recovery across Android, the web client and future clients.
The protocol must make another client implementable without copying undocumented
behaviour from the existing apps.

## Contract, protocol and encoding

Proto is the contract layer: operations and concrete message types shared by the
server and clients. Generated Go/Java code is useful without making binary RPC a
requirement. MUCP specifies how operations are addressed and messages correlated,
including errors and lifecycle rules. HTTPS is the first external transport; JSON
is its human-readable body encoding.

```text
               proto/assistant/v1
              shared types + operations
                 /             \
        Go implementation    Java client types
                 \             /
                  MUCP messages
                 HTTPS + JSON
                       |
            handwritten clients also work
```

Use the standard protobuf JSON mapping for the new binding: lowerCamelCase keys,
string enum names, RFC3339 timestamps, base64 bytes and quoted 64-bit integers.
Unknown response fields must not prevent rendering known text/fallbacks. Unknown
request access modes must be rejected. A handwritten client can send the same JSON
without protobuf tooling. The existing HTTP JSON remains a separate compatibility
surface; it must not be silently reinterpreted as protobuf JSON.

The default public binding will not require binary framing, a service registry,
goroutines, gRPC or go-micro internals in clients. Reuse go-micro's metadata and
implementation where compatible, and put any required adapter at the authenticated
server boundary. Internal service-to-service RPC remains a separate concern.

## Existing foundation

The reference implementation is the go-micro version pinned in Mu's `go.mod`.
Relevant upstream packages are `client`, `server`, `codec`, `transport`,
`transport/headers` and `broker`. The default RPC client/server implement MUCP;
JSON and protobuf body codecs already exist. Publishing uses the broker
abstraction; its delivery guarantees depend on the broker.

Existing metadata includes:

| Header | Meaning |
| --- | --- |
| `Micro-Service` | Target service |
| `Micro-Endpoint` | Target endpoint |
| `Micro-Method` | Method metadata as interpreted by the implementation |
| `Micro-ID` | RPC message identity/correlation |
| `Micro-Stream` | Active stream identity |
| `Micro-Error` | RPC error |
| `Micro-Topic` | Published message topic |
| `Content-Type` | Body encoding |

This table documents the internal implementation, not a finished external wire
specification. Verify existing header values and semantics before reusing them.
Internal binary framing is not a requirement for the external JSON binding. In particular, do not
assume the historical proposal's `Micro-Method: Call` vocabulary matches the
current codec. HTTP header names are case-insensitive; envelope keys need an
explicit compatibility rule. The historical proposal is background, not the
interoperability test:
<https://gist.github.com/asim/9db0b487505991d75de79eef084ba23e>.

Mu currently uses a shared in-memory transport and broker for internal services.
The internal listener must not simply be opened to external clients: account
metadata supplied by a caller is not authentication.

## Assistant contract

The initial contract is `proto/assistant/v1/assistant.proto`, with protobuf package
`mu.assistant.v1`. It defines ListAgents, ListThreads, ReadThread and unary Ask with typed results
and explicit personal/public access modes. All four operations are registered.
The table also includes later lifecycle work, which is not yet supported.
`proto/` owns public wire definitions, not runtime services or UI navigation.
Generate Go and Android Java bindings from the same schema. Specify protobuf
JSON mapping for browser clients, including enum, timestamp and integer handling.
Do not treat the current hand-written JSON API as automatically protobuf JSON.

| Operation family | Required behaviour |
| --- | --- |
| Capabilities | Supported contract versions, operations and content types |
| Agents | Accessible agents and public descriptions; never private instructions |
| Threads | List and read personal assistant threads; Ask creates or continues one |
| Requests | Submit input and acknowledge an identifiable operation |
| Responses | Text, typed results, progress and an explicit terminal outcome |
| Cancellation | Request cancellation and report its actual outcome |
| Updates | Resume authorised conversation/operation events after disconnection |

The schema defines RPC names and field numbers for the initial operations.
Use `thread`, matching the existing API and store; the UI may say conversation.
Thread IDs belong in message bodies because their lifetimes extend beyond an RPC.
Operation IDs and durable completion are future lifecycle work. Account identity comes from verified credentials. Existing
MUCP metadata stays in headers rather than being duplicated in every body.

Responses contain Markdown text and typed content blocks using protobuf `oneof`:
sources, images, audio, video, routes and resource references. Confirmation
requests remain future work because they need server-held action state. Include a text/link fallback so
older clients can present an unfamiliar block. Media references and actions are
data, never model-authored executable HTML. Media URLs remain subject to client
validation and provider playback restrictions. Confirmations refer to server-held
operations; a client must not manufacture tool authority from a rendered action.

The runtime owns reasoning, tools, permissions, billing and persistent history.
Clients own rendering, microphone input and playback. Speech recognition and
synthesis can remain local; the initial contract does not require raw audio.

## Behaviour to specify before v1

- Separate accepted, running and terminal outcomes. Define how the final message
  relates to operation completion and how failures are represented.
- Define explicit idempotency keys, account scope, retention and conflict handling.
  An RPC message ID alone does not make retries safe or prevent duplicate charges.
  Until implemented, clients must not automatically retry ambiguous mutations.
- Define event sequence/cursor scope, retention and recovery when a cursor expires.
  A stream or broker subscription alone does not provide durable replay.
- Separate stopping local playback from cancelling server work. Cancellation is
  best effort; it cannot undo an email already sent or another committed action.
- Check access on every operation and subscription, including membership revocation.
  Authenticate at the public boundary, derive trusted account context there, and
  preserve the existing tool, quota and billing checks during dispatch.
- Define schema evolution, unsupported operations/content and capability negotiation.
  Reserve removed protobuf field numbers; do not reuse them for new meanings.

## HTTPS binding: mucp/1

`GET /` with `Micro-Protocol: mucp/1` returns public capabilities without account data.
Ordinary requests without the protocol header retain the landing/Home behaviour.
The protocol header selects dispatch at the root; unknown versions fail explicitly.
`/mucp` remains a compatibility alias, including its headerless GET discovery. Calls are `POST /`
with `Content-Type: application/json` and exactly one of each header:

| Header | Value |
| --- | --- |
| Micro-Protocol | `mucp/1` |
| Micro-Service | `assistant` |
| Micro-Endpoint | `Assistant.ListAgents`, `Assistant.ListThreads`, `Assistant.ReadThread` or `Assistant.Ask` |
| Micro-Id | Caller-generated 16–128 ASCII letters, digits, hyphens or underscores |
| Authorization | `Bearer <token>` for native/programmatic clients |

The built-in runtime web experience may use same-origin session cookies and
`X-CSRF-Token`. The standalone web client uses OAuth bearer tokens with cookies
omitted, including when its build is served by Mu.
An explicit token overrides cookies. Reads require read access; Ask requires write
access, with `api:agent` for scoped product tokens. Service-only tokens cannot run
agents. Identity is derived from credentials; caller-supplied account metadata is
never forwarded to internal go-micro services.

Example body for `Assistant.Ask`:

```json
{"agentId":"micro","thread":"","prompt":"What is the weather?","mode":"ACCESS_MODE_PERSONAL","voice":false}
```

Responses echo protocol, service, endpoint and correlation ID. Successful responses
are protobuf JSON. Requests reject unknown fields and unknown access modes; bodies
are limited to 64 KiB. Clients must tolerate additive response fields. No SDK is
required. Native Java and generated Go types use the same JSON mapping.

Failures use non-2xx HTTP status, `Micro-Error`, and a JSON `error` object with
`code` and `message`. Authentication/authorization failures retain the existing
product API error body. Important statuses are 400 invalid arguments, 401 sign-in,
403 permission/verification, 404 inaccessible resource, 402 credit, 409 acceptance
conflict, 413 body size, 415 encoding, 429 rate limit and 504 pending outcome.
There is no automatic mutation retry or fallback to a legacy endpoint.

PERSONAL Ask submits to the existing durable reply queue and waits up to 120 seconds.
Disconnecting stops that HTTP wait, not execution. A known accepted thread is also
returned in `Micro-Thread`, including pending-outcome errors. If acceptance was lost,
use ListThreads to find the new chat. ReadThread returns messages and `pending`;
clients can poll it and recover the saved reply. PUBLIC remains an authenticated,
metered, stateless restricted question and does not create a recoverable thread.

Request IDs correlate exchanges. The existing personal queue deduplicates retained
submissions, but this is not a general, permanent idempotency guarantee. Never
resend an ambiguous Ask automatically. Restarted in-flight work records interruption
rather than blindly replaying tools. Cancellation is not implemented.

Pagination tokens are signed, scoped to account/operation/filter, and expire after
15 minutes or a runtime restart. Restart pagination on an expired token, deduplicating
by record ID. These are live pages, not snapshots. ReadThread is oldest-first.

Android and Mu Web offer agents, Chats, New chat and paginated history through
MUCP. The static web client renders Markdown and typed results locally; it does
not fetch server-rendered chat pages. The broader runtime web experience retains
its existing API. Browser clients use OAuth with S256 PKCE and explicit scoped bearer tokens.
The runtime permits non-credentialed CORS only at the MUCP and OAuth
registration/token endpoints, never dashboard pages. No account authority is
granted by CORS; MUCP still enforces the token permissions and account ownership.

Go types are committed and checked for generation drift. Java types are generated
by the Android build from the same proto. The adapter is implemented in `agent/`,
registered by `internal/server`, and calls the existing agent execution path. It
does not expose the internal RPC listener or introduce another agent/tool loop.

Assistant-specific requirements stay in Mu. Changes to go-micro must be justified
by general framework needs. This work does not require a new broker, network
registry, payment protocol or replacement for the existing service API and MCP.

Current client usage is documented at [Developers](/developers). See
[Architecture](ARCHITECTURE.md) for the runtime boundaries.
