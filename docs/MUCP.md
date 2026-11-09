# Micro Communication Protocol (MUCP)

Status: design draft. Mu already uses go-micro's RPC client/server internally.
The initial assistant protobuf interface is drafted; native/browser MUCP bindings
remain proposed; existing clients continue to use the current HTTPS API.

## Purpose

MUCP carries service requests, responses, streams and events independently of
the transport and body encoding. Reuse go-micro's implementation rather than
introducing another RPC stack. Mu defines the assistant contract carried by it;
clients present its results using their own platform controls.

A conventional HTTP API remains sufficient for individual requests. The value
here is a shared, documented contract for conversations, structured results,
completion, cancellation and recovery across Android, the PWA and future clients.
The protocol must make another client implementable without copying undocumented
behaviour from the existing apps.

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

This is not a new wire specification. Preserve the implementation's actual
header names, values, framing and end-of-stream semantics. In particular, do not
assume the historical proposal's `Micro-Method: Call` vocabulary matches the
current codec. HTTP header names are case-insensitive; envelope keys need an
explicit compatibility rule. The historical proposal is background, not the
interoperability test:
<https://gist.github.com/asim/9db0b487505991d75de79eef084ba23e>.

Mu currently uses a shared in-memory transport and broker for internal services.
The internal listener must not simply be opened to external clients: account
metadata supplied by a caller is not authentication.

## Assistant contract

The initial draft is `proto/assistant/v1/assistant.proto`, with protobuf package
`mu.assistant.v1`. It defines ListAgents and unary Ask with typed results and
explicit personal/public access modes. No public binding is implemented yet.
The remaining operation families below are the roadmap, not declared RPCs.
`proto/` owns public wire definitions, not runtime services or UI navigation.
Generate Go and Android Java bindings from the same schema. Specify protobuf
JSON mapping for browser clients, including enum, timestamp and integer handling.
Do not treat the current hand-written JSON API as automatically protobuf JSON.

| Operation family | Required behaviour |
| --- | --- |
| Capabilities | Supported contract versions, operations and content types |
| Agents | Accessible agents and public descriptions; never private instructions |
| Conversations | Create, list, resume and retrieve authorised messages |
| Requests | Submit input and acknowledge an identifiable operation |
| Responses | Text, typed results, progress and an explicit terminal outcome |
| Cancellation | Request cancellation and report its actual outcome |
| Updates | Resume authorised conversation/operation events after disconnection |

The schema defines RPC names and field numbers for the initial two operations.
Conversation and operation IDs belong in message bodies because their lifetimes
extend beyond an RPC. Account identity comes from verified credentials. Existing
MUCP metadata stays in headers rather than being duplicated in every body.

Response content should use typed blocks with protobuf `oneof`: text, sources,
images, audio, video and confirmation requests. Include a text/link fallback so
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

## Transport and implementation plan

Start with HTTPS. Test go-micro's existing HTTP framing and streaming against a
native Android client and browser APIs before selecting a binding. A browser
adapter may be necessary; ordinary HTTP support does not establish compatibility
with browser bidirectional streaming. Document framing, authentication, errors,
stream termination and supported operations for each binding. Do not advertise
transport independence as identical capabilities on every transport.

1. Inventory the existing client requests and typed results; preserve their routes
   while introducing a compatible adapter over the same execution path.
2. Define the protobuf assistant schema and the verified HTTPS binding.
3. Prove an authenticated request, structured response and cancellation end to end
   against Mu using go-micro's existing implementation.
4. Add shared wire fixtures and conformance tests for Android and browser clients,
   including permission failures, unknown content, duplicate submission and
   disconnect recovery before claiming those behaviours are supported.
5. Migrate both clients incrementally. Add further transports only for a concrete
   client requirement.

Assistant-specific requirements stay in Mu. Changes to go-micro must be justified
by general framework needs. This work does not require a new broker, network
registry, payment protocol or replacement for the existing service API and MCP.

Current client usage is documented at [Developers](/developers). See
[Architecture](ARCHITECTURE.md) for the runtime boundaries.
