<!-- SPDX-License-Identifier: Apache-2.0 -->

# Protocol contracts

`mucp/v1` defines the transport-independent envelope: canonical metadata headers
and encoded body bytes. The HTTPS adapter uses this generated type internally,
while carrying headers natively and sending the JSON body without a wrapper.
Credentials are binding context, not envelope metadata.

`assistant/v1` defines the client composition surface: Home, Inbox, Assistant
(agent discovery, personal threads and execution), and Services (catalogue).
`service/<name>/v1` defines contracts for all 40 existing services. Chat, Groups, Notes, Tasks, Bookmarks and Alerts use their generated request/response types in the service
implementations and are exposed through MUCP alongside Contacts. The other
contracts are draft schemas derived from existing service RPC types, not a claim
that those operations are available over MUCP. Mu adapts these to
existing packages; there is no second assistant engine or `service/assistant`.

The shared HTTPS/JSON binding lives in `internal/mucp`; `internal/server` composes
explicit adapters from home, inbox, agent and services. `/` detects MUCP headers;
ordinary browser traffic continues to use the runtime web experience.
Android generates Java types; web/desktop and the independent Python example use
readable JSON. See the [HTTPS binding](../docs/MUCP-HTTP.md) for wire semantics and the
[Mu runtime profile](../docs/MUCP-RUNTIME.md) for authorization and recovery.
The [conformance guide](../docs/MUCP-CONFORMANCE.md) covers the independent client
and live agent-to-agent checks.

## Licence

The contracts, generated types, examples and fixtures in this directory are
licensed under [Apache-2.0](LICENSE). They may be used in open-source or proprietary
clients and runtimes under those terms. Include the licence when redistributing
them and retain applicable notices. Protobuf libraries and other dependencies
retain their own licences.

**Exception:** `generate/` remains [AGPL-3.0](generate/LICENSE). Its schema output
is Apache-2.0; running the generator does not change the output's licence.
The Mu runtime and existing client implementations retain the repository's AGPL
licence. See the [licensing map](../LICENSING.md) for the full boundary.

## Contract and wire format

Proto supplies concrete types and operation signatures. The external MUCP
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
  must retain distinct outcomes. Wire errors are documented in MUCP.
- Submit provides a durable operation ID and seven-day minimum receipt window.
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
| ListThreads / ReadThread | Internal thread store and existing web chat views | Authorised personal-history projection and opaque pagination and pending recovery |
| PERSONAL / PUBLIC | `public_only` | Explicit mode validation and capability check |
| voice | `context.voice` | Presentation hint only |
| content | `results` | Convert known kinds; preserve other resources as safe references |

The adapter calls the existing execution path, rather than making a second
agent loop or bypass account, tool and billing gates. The legacy routes stay
compatible while clients migrate. Chat and Groups bind the same generated types through MUCP and the existing service gateway. Contacts retains its explicit pagination adapter; other services migrate incrementally.

## Validation and generation

From the repository root, with protoc 31.1 (also bundled in grpcio-tools 1.76.0):

```sh
protoc -I proto --descriptor_set_out=/tmp/mu-assistant.pb --include_imports \
  proto/mucp/v1/message.proto proto/assistant/v1/assistant.proto
```

CI compiles the schema and checks committed Go types for drift. Android generates
Java types at build time using protoc 4.31.1 (compiler 31.1).
For Go, use protoc-gen-go v1.36.11 with `--go_out` and
`--go_opt=paths=source_relative`. That generates message types, not gRPC or MUCP
handlers. RPC glue must use go-micro's compatible generator or an adapter proven
against its pinned version; declaring `service` does not mandate gRPC.

Wire fixtures and authenticated integration checks guard the implemented binding. Once stable, retain field numbers, reserve deleted names
and numbers, and make compatible additions without changing existing meanings.

## Service schema maintenance

Public service contracts are authored in proto. Implement their generated Go
request/response types; do not maintain duplicate RPC structs or regenerate the
public schema from Go. Storage records remain an implementation detail.

For the remaining **unpublished draft schemas only**, run
`go run ./proto/generate` from the repository root after changing service RPC
request/response types, then regenerate Go types with protoc. CI runs
`go run ./proto/generate -check`, compiles every schema, and checks generated Go
code for drift. The generator reads each existing `Spec.Handler` and endpoint
description without starting services or exposing new endpoints. A coverage test
requires every service to be included. Chat, Groups, Notes, Tasks, Bookmarks and Alerts are now proto-first: edit their source `.proto` files and
regenerate their Go types, rather than deriving schemas from Go. Their handlers
use generated types directly (local type aliases preserve package call sites).
Storage records remain separate and are projected into public response messages.
Contacts retains its explicit published pagination contract.

`service/fields.json` is the field-number ledger. Keep it in version control:
reordering Go fields must not renumber the wire contract; deleted numbers are
reserved, and changing an existing field's type fails generation. New operations
or adapters still require semantic review, authorization and interoperability
tests. Generated schemas describe existing API shapes, including legacy action
strings; they do not replace runtime validation with protobuf validation.

The transport uses protobuf JSON mapping, not arbitrary `encoding/json` output:
64-bit integers encode as strings, bytes as base64, and timestamps as RFC 3339.
Nested containers use a named `values` wrapper. Explicit `json_name` preserves
existing field spellings where possible. An adapter must handle these differences
and apply the existing service gateway, scopes, account checks and billing.
Required-input and description options in `service/options.proto` keep MCP tool
metadata and generated SDK requiredness aligned with proto-first contracts.
Only registered MUCP operations appear in discovery. Schema-only services remain
accessible through their existing REST/MCP interfaces until an adapter ships.
