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

The initial draft is `proto/assistant/v1/assistant.proto`, with protobuf package
`mu.assistant.v1`. It defines ListAgents, ListThreads, ReadThread and unary Ask with typed results
and explicit personal/public access modes. No public binding is implemented yet.
The table includes both draft operations and later lifecycle work; none is a
registered public MUCP endpoint yet.
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

## Transport and implementation plan

Start with human-readable JSON over HTTPS. Test which go-micro metadata and
dispatch conventions can be reused by native Android and ordinary browser APIs. A browser
adapter may be necessary; ordinary HTTP support does not establish compatibility
with browser bidirectional streaming. Document framing, authentication, errors,
stream termination and supported operations for each binding. Do not advertise
transport independence as identical capabilities on every transport.

1. Inventory the existing client requests and typed results; preserve their routes
   while introducing a compatible adapter over the same execution path.
2. Validate the draft schema against existing agent/thread behaviour and specify
   exact URL/header, JSON, error and version-negotiation rules for the HTTPS binding.
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
