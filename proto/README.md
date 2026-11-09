# Protocol contracts

`assistant/v1/assistant.proto` is the initial **draft** assistant interface.
It declares ListAgents, ListThreads, ReadThread and Ask. Clients discover an agent,
list/read its personal threads, then ask a question using the existing `thread`
identifier or omit it to start a new thread. The implementation belongs in
`agent/`; server composition will register its authenticated MUCP boundary.
There is no `service/assistant` package and no public MUCP listener yet.

The schema compiles, but existing Android/PWA clients still use their current
HTTPS routes. These messages are not a drop-in replacement for that JSON.
See [MUCP](../docs/MUCP.md) for architecture and the binding work still required.

## Contract and wire format

Proto supplies concrete types and operation signatures. The planned external MUCP
binding uses HTTPS and human-readable JSON with protobuf JSON mapping. Generated
Go/Java code is optional for clients; binary RPC and gRPC are not requirements.
The draft rename from `conversation_id` to `thread` retains the field numbers;
no public binding or client has shipped using the former JSON name.

## Behaviour

- Credentials determine the caller; no request accepts an account identity.
- `ListAgents` is account-scoped and exposes names/descriptions, not instructions.
  IDs are opaque selectors, not display names or assumptions about URL slugs.
- `Ask` returns one completed answer. A PERSONAL request creates or resumes a
  conversation owned by the caller and bound to the selected agent. Omitting
  `agent_id` selects the runtime default; it does not select a foreign agent from
  the conversation. The adapter must validate both ownership and agent binding.
- ListThreads/ReadThread expose only the caller's personal assistant history and
  accessible agents, excluding mail, group threads and internal execution records.
  Listing is ordered by updated time descending then ID ascending; message reads
  by time then ID ascending. Reads do not mark threads seen. Page tokens are
  opaque and bound to identity, operation and filter/thread, rechecked on every
  call. Invalid/expired tokens are errors, not silently restarted pages. Live
  pagination is not a frozen snapshot; clients deduplicate by ID.
- Access mode is explicit. Unknown or unspecified values fail closed. PUBLIC is
  still authenticated and billed but excludes private context and does not retain
  a conversation. Discover support before use; never fall back to PERSONAL.
- Nonblank input is bounded to 8,000 UTF-8 bytes. The new adapter must reject
  oversized input rather than copy the legacy HTTP handler's truncation behaviour.
- RPC errors use MUCP's error mechanism, not success-shaped response bodies.
  Authentication, permission, missing-resource, invalid-input and quota failures
  must retain distinct outcomes. Exact wire error codes await binding tests.
- There is no idempotency or durable operation ID in this initial interface.
  Do not retry an ambiguous Ask automatically: execution may have taken place.
  A disconnect does not prove cancellation, rollback or a refund.
- Render `text` as safe Markdown. Content blocks are structured data, never HTML
  to execute. An unknown or absent `oneof` uses its required plain-text fallback.
  In protobuf JSON, readers must tolerate unknown response fields to reach it;
  request enum validation must still reject unknown access modes.
- Resolve relative resource URLs against the configured runtime origin. Only
  validated HTTPS URLs may be opened externally; never forward runtime credentials
  to other origins. Validate media IDs, MIME types and provider restrictions.
  A video result alone does not authorise autoplay or a hidden YouTube player.

Streaming, cancellation, subscriptions and confirmation
operations are deliberately not declared until their lifecycle and permission
semantics are implemented. Adding an RPC declaration does not make it supported.

## Existing API mapping

| Contract | Current interface | Adapter work |
| --- | --- | --- |
| ListAgents | `GET /agent?view=agents` | Public descriptors and `public_voice` capability |
| Ask | `POST /agent/<slug>` | Resolve opaque agent ID; retain existing execution/permissions |
| thread | `thread` | Ownership and selected-agent binding |
| ListThreads / ReadThread | Internal thread store and existing web chat views | Authorised personal-history projection and opaque pagination; no equivalent public MUCP endpoint yet |
| PERSONAL / PUBLIC | `public_only` | Explicit mode validation and capability check |
| voice | `context.voice` | Presentation hint only |
| content | `results` | Convert known kinds; preserve other resources as safe references |

The future adapter must call the existing execution path, not make a second
agent loop or bypass account, tool and billing gates. The legacy routes stay
compatible while clients migrate. Tool-service contracts are outside this change.

## Validation and generation

From the repository root, with protoc 31.1 (also bundled in grpcio-tools 1.76.0):

```sh
protoc -I proto --descriptor_set_out=/tmp/mu-assistant.pb --include_imports \
  proto/assistant/v1/assistant.proto
```

CI compiles the schema to a descriptor set.
Generated client bindings are not committed until a client consumes them.
For Go, use protoc-gen-go v1.36.11 with `--go_out` and
`--go_opt=paths=source_relative`. That generates message types, not gRPC or MUCP
handlers. RPC glue must use go-micro's compatible generator or an adapter proven
against its pinned version; declaring `service` does not mandate gRPC.

Before publication as stable v1, add wire fixtures and authenticated integration
checks for both clients. Once stable, retain field numbers, reserve deleted names
and numbers, and make compatible additions without changing existing meanings.
