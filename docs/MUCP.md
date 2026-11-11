<!-- SPDX-License-Identifier: Apache-2.0 -->

# Micro Communication Protocol (MUCP)

## Status and purpose

MUCP defines client-to-server communication for addressed, typed operations.
Clients may be applications, scripts or agents. Services and assistants define
the operations carried by the protocol. Mu (short for Micro) is a runtime using
it; Mu's assistant and service contracts are separate from the communication core.

This is a working specification of `mucp/1`, not an IETF submission or an adopted
Internet Standard. Its implemented binding is JSON over HTTPS. The goal is that
independent implementations can interoperate without reading Mu's source code.
Known gaps are recorded rather than silently treated as conformance requirements.

The key words MUST, MUST NOT, REQUIRED, SHOULD, SHOULD NOT and MAY are interpreted
as in BCP 14 ([RFC 2119](https://www.rfc-editor.org/rfc/rfc2119) and
[RFC 8174](https://www.rfc-editor.org/rfc/rfc8174)) only when capitalized.

## Document set

| Document | Defines |
| --- | --- |
| This core | Roles, addressing, correlation, contract boundaries and evolution |
| [HTTPS binding](MUCP-HTTP.md) | Exact requests, discovery, responses and errors for `mucp/1` |
| [Mu runtime contracts](MUCP-RUNTIME.md) | Assistant operations, durable work, Home, Inbox and direct services |
| [Conformance](MUCP-CONFORMANCE.md) | Executable checks, live-instance testing and unproven requirements |

The specification and schemas must be used at the same pinned repository
revision. There is not yet a separately released, immutable specification bundle.

## Roles and boundaries

A **client** originates an exchange. A **server** authenticates the caller, routes
the operation and returns a result or failure. An **agent** may act in either role.
A **service** is an addressable collection of operations. A **contract** defines
their typed requests, results, authorization requirements and behaviour.

Speaking MUCP and implementing a particular contract are separate claims. A server
can speak MUCP without implementing personal assistants. A Mu-compatible assistant
server additionally implements the named Mu contract and its semantics.

Go-micro is intended to provide a reusable client/server implementation of this
communication layer. Today the public HTTPS binding is implemented in Mu's
`internal/mucp`; moving it into go-micro is subsequent work. Historical go-micro
MUCP transports are not automatically compatible with this binding. Implementers
in other languages do not need Go, go-micro or Mu implementation packages.

## Addressing and messages

An invocation identifies a protocol version, service, endpoint and correlation
identifier. The pair (service, endpoint) selects an operation in a contract.
Names are case-sensitive and MUST NOT be interpreted as executable code, file
paths or instructions to expose an internal registry. The server MUST explicitly
support an operation before invoking it. Discovery is not authorization.

Each message consists of metadata and an encoded body. The
[envelope schema](https://github.com/micro/mu/blob/main/proto/mucp/v1/message.proto)
represents metadata as lowercase singleton keys and the body as bytes. It does
not require bindings to serialize an extra wrapper; the HTTPS binding uses
native HTTP headers and an unwrapped JSON body.

The contract supplies the body schema. This version uses protobuf JSON mapping;
clients MAY encode JSON without generated code. Schema field numbers and internal
Go/Java package names are not operation addresses. The core imposes no assistant,
conversation, credit balance, model or application navigation model.

Credentials and authenticated identity belong to the binding's security context.
They MUST NOT be copied into the message metadata or passed onward to tools as
protocol metadata. Caller-supplied account IDs MUST NOT grant authority.

## Exchange semantics

A correlation ID associates a request and its response. It MUST be echoed on a
successful invocation. It is NOT an idempotency key. Clients SHOULD use a new ID
for each exchange, including an explicitly permitted retry of the same operation.

A client MUST verify protocol version and correlation before accepting a successful
invocation response. A transport failure is an unknown outcome, not evidence
that the operation did not execute. Closing a connection does not cancel work.
Clients MUST NOT automatically replay an invocation unless its contract explicitly
permits the retry and defines its deduplication or idempotency semantics.

An operation contract MUST define inputs, outputs, access rules, observable
side effects, failures and whether/how a caller can recover an uncertain outcome.
Durable operation IDs, retention and replay windows are contract-level concepts.
They are not inferred from correlation IDs or a reliable transport.

## Discovery and extensions

Servers expose their supported contract through binding-specific discovery.
Clients MUST NOT infer permission from an advertised capability. Absence of a
feature means it is not established as supported. A client MUST reject an
incompatible version rather than silently changing protocols or destinations.

In `mucp/1`, discovery names one primary service/contract. Mu uses its Services
contract for additional service discovery. Generic multi-contract discovery,
contract-version selection and a namespace allocation policy remain open design
work. This draft does not reserve arbitrary third-party service names globally.

Additive response fields are compatible; clients MUST tolerate them. Requests
are validated against the selected schema: unknown request fields are rejected.
Therefore adding an optional request field does not make it safe to send to an
older server without establishing support. A changed field meaning, required
header or operation meaning requires an explicit versioned contract/binding.
Protobuf field numbers MUST NOT be reused after deletion.

Streaming, cancellation, subscriptions and other transport bindings are not
defined by `mucp/1`. A future binding must define framing, security, ordering,
failures and limits, and preserve the applicable contract semantics. Transport
independence is a design boundary, not a claim that every transport works today.

## Security and privacy considerations

The network may observe, modify, redirect or replay unprotected traffic. Clients
and servers must also treat peers and returned content as potentially malicious.
HTTPS authenticates the server and protects traffic; application authorization
still determines what a caller can do. A malicious authorized caller must not
obtain another principal's history, work or service records.

Credentials MUST be scoped to the selected server and MUST NOT be forwarded to a
redirect target, a result URL, another agent or a tool. Discovery and model output
MUST NOT confer permissions. Servers MUST recheck authorization when recovering
work; possession of an operation ID is not authorization. Applications MUST treat
returned text/content as data, never executable authority.

Implementations need bounds on input size, parsing, execution and recovery reads.
Repeated status polling can exhaust a server even without model calls. Retry
scheduling belongs in the applicable binding/contract and must be bounded; a
pending state is not permission for unlimited rapid polling.

Prompt and credential contents MUST NOT be put in URLs. Logging and retention
policies must account for private request/response data. TLS does not protect
against a compromised runtime or an external provider to which the runtime
legitimately sends data. Federated identity, onward delegation and cross-runtime
trust are outside this version's scope.

## Registrations and references

No IANA registrations are requested by this repository draft. Before an Internet-
Draft submission, review HTTP field registration, namespace/extension policy and
IETF publication requirements; do not describe these private fields as registered.
See [RFC 8126](https://www.rfc-editor.org/rfc/rfc8126) for registry policy guidance
and [RFC 3552](https://www.rfc-editor.org/rfc/rfc3552) for security review guidance.
Normative wire dependencies are listed in the HTTPS binding.

Licensed under [Apache-2.0](../proto/LICENSE).
