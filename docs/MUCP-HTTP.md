<!-- SPDX-License-Identifier: Apache-2.0 -->

# MUCP HTTPS/JSON binding

Status: working specification of `mucp/1`. Read with the [core](MUCP.md).
Requirement IDs below are referenced by the [conformance plan](MUCP-CONFORMANCE.md).
This document defines the wire binding, not an assistant or service implementation.

## 1. Endpoint and transport (HTTP-01)

A configured server origin identifies the endpoint: `https://example.net/`.
HTTPS is REQUIRED except for local development over HTTP on loopback. Clients
MUST validate TLS certificates and MUST NOT follow redirects for authenticated
calls. This binding uses HTTP semantics from RFC 9110; it does not define a new
TCP protocol or require HTTP/1.1 rather than HTTP/2 or HTTP/3.

`GET /` with `Micro-Protocol: mucp/1` discovers the primary contract.
`POST /` invokes an operation. Discovery does not require credentials. Invocation
requires authentication as defined by the server's security profile. Other
methods return 405 with `Allow: GET, POST` after MUCP routing. Mu retains `/mucp`
as a compatibility alias. Requests without the protocol header at `/` belong to
the website and are outside this binding, not an anonymous invocation fallback.

## 2. Invocation metadata (HTTP-02)

| Header | Requirement |
| --- | --- |
| `Micro-Protocol` | Exactly `mucp/1` |
| `Micro-Service` | Exact service name defined by the selected contract |
| `Micro-Endpoint` | Exact endpoint name defined by the selected contract |
| `Micro-Id` | 16–128 ASCII letters, digits, hyphens or underscores |
| `Content-Type` | `application/json`; media-type parameters may be present |

All five headers MUST occur once. HTTP field names are case-insensitive; routing
values are case-sensitive. A duplicate field line MUST be rejected, including
Content-Type. A comma-joined routing value is not a list of alternatives. Unknown
service/endpoint pairs return 404, not a successful empty result. Unknown protocol
versions return 400. No general service-name grammar is imposed beyond valid HTTP
field values and exact matching to a supported contract in this version.

Missing/duplicate routing fields return 400 `invalid_headers`. Missing or invalid
Content-Type returns 415 `unsupported_encoding`; duplicate Content-Type returns
400 `invalid_headers`. Invalid IDs return 400 `invalid_id`.

Other HTTP fields do not establish identity or modify operation meaning. A contract
extension must document any additional protocol fields; arbitrary headers MUST NOT
be propagated as tool credentials or instructions.

## 3. Body encoding (HTTP-03)

The body MUST be one UTF-8 JSON object using the selected contract's protobuf JSON
mapping. The limit is 65,536 body bytes, inclusive. This is the unwrapped request
message, not a serialized metadata/body envelope. This binding defines no
compressed-request-body support; clients send uncompressed JSON.

Protobuf JSON defines lowerCamelCase names (or explicit json_name), string-encoded
64-bit integers, base64 bytes and RFC 3339 timestamps. It also defines accepted
proto field-name aliases, omitted defaults, null values and enum representations.
A contract may impose stricter semantic validation, such as rejecting unknown or
unspecified access modes. Unknown fields, duplicate fields, multiple objects,
invalid JSON and a non-object root MUST be rejected with 400 `invalid_arguments`.
An oversized body returns 413 `too_large`. Authentication may reject a call before
its body is parsed; clients MUST NOT depend on an error precedence for requests
with multiple independent faults.

Responses use the same protobuf JSON mapping. Clients MUST tolerate additive
response fields and MUST NOT depend on field ordering or whitespace. A contract
specifies any fallback for unknown result variants. Implementations may include
or omit default-valued fields in accordance with protobuf JSON semantics.

## 4. Discovery (HTTP-04)

Successful discovery is HTTP 200 with application/json and `Micro-Protocol: mucp/1`.
It has no invocation correlation ID. The JSON object has these fields:

| Field | Type and meaning |
| --- | --- |
| `protocol` | String, exactly `mucp/1` |
| `service` | Nonempty string, primary routing service |
| `contract` | Nonempty string, versioned contract identifier |
| `encoding` | String, `protobuf-json` |
| `operations` | Array of endpoint strings supported for the primary service |
| `features` | Optional object mapping feature names to booleans |
| `limits` | Optional object mapping limit names to nonnegative integers |

Unknown response fields/features/limits MUST be tolerated. Missing optional fields
MUST NOT be interpreted as support. Operation order has no meaning. Discovery
MUST NOT contain account-private data and MUST NOT imply authorization. Secondary
service discovery is contract-defined; this version does not pretend the primary
operations array inventories every service on the instance.

Mu's discovery values and authenticated service catalogue are defined in the
[runtime profile](MUCP-RUNTIME.md#discovery-profile). A different implementation
can expose a different primary service and contract. Mu's current handler still
hard-codes its primary discovery profile; extracting generic discovery into
configurable go-micro code is follow-up implementation work.

## 5. Invocation responses (HTTP-05)

A successful invocation MUST return HTTP 200, application/json, a JSON object and
exact echoes of Micro-Protocol, Micro-Service, Micro-Endpoint and Micro-Id.
Clients MUST verify protocol and correlation before accepting the result. Response
metadata is not repeated in the body. Mu sets `Cache-Control: private, no-store`;
servers implementing this binding MUST prevent shared caching of private results.

An error before valid routing/correlation is established may omit echoes.
Clients MUST handle non-2xx status even if a proxy returns an unstructured body.
A response that cannot be validated is an uncertain outcome, never proof that a
mutation was not executed. There is no automatic redirect, version downgrade or
retry mechanism in the binding.

## 6. Authentication boundary (HTTP-06)

Bearer calls use `Authorization: Bearer <token>` and do not carry cookies.
Credentials are obtained out of band or through a separately specified OAuth
profile. A valid-looking routing header or body field does not authenticate a
caller. Servers MUST reject invalid explicit credentials rather than falling
back to another identity such as a cookie session.

Cookie authentication, OAuth bootstrap, exact scope names, CSRF and CORS policy
are deployment/profile concerns. Mu's profile defines them in
[accounts and sign-in](MUCP-RUNTIME.md#accounts-and-sign-in). A client cannot
assume Mu's OAuth URLs or token lifetimes apply to an arbitrary MUCP server.
Authorization MUST be checked per operation and resource, including recovery reads.

## 7. Errors (HTTP-07)

A binding-generated error is a non-2xx HTTP response with application/json,
`Micro-Error: <code>` and the object:

```json
{"error":{"code":"invalid_arguments","message":"Invalid request","status":400}}
```

`code` is a machine-readable string; `message` is display text; `status` equals the
HTTP status. Unknown codes MUST be handled using HTTP status without replaying a
mutation. Error messages MUST NOT be parsed to decide behaviour or executed as HTML.

| Status | Binding codes / meaning |
| --- | --- |
| 400 | `invalid_headers`, `unsupported_protocol`, `invalid_id`, `invalid_arguments` |
| 401 | `unauthenticated`; obtain valid credentials |
| 403 | `forbidden`, `csrf`; authority insufficient |
| 404 | `not_found`; unknown operation or inaccessible resource |
| 405 | `method_not_allowed` |
| 413 | `too_large` |
| 415 | `unsupported_encoding` |
| 429 | `rate_limited`; do not automatically replay a mutation |
| 500 | `execution_failed`, `encoding_failed`; outcome may be uncertain |

Contracts add operation-specific codes/statuses. Mu uses 402 for credits, 409 for
submission conflicts and 504 for an unfinished synchronous wait. These are not
requirements to implement a payment system or an assistant in every MUCP server.

Compatibility exception: Mu's existing product authorization gate sets
`Micro-Error: authorization_failed` but may return a string-valued `error` rather
than the object above. Clients MUST handle that response by status. Uniform
structured authentication errors are a known gap, not a claim this PR fixes.
Intermediary failures may omit both the code and JSON body entirely.

## 8. Recovery and flow control (HTTP-08)

The binding does not make POST idempotent. Only an operation contract can permit
replay. A dropped connection or 504 does not establish acceptance or rejection.
Read/recovery calls SHOULD use bounded polling with backoff on transient failures;
clients MUST stop at a terminal result or their local deadline. A deadline on
polling does not cancel server work. This version has no negotiated polling
interval or uniform Retry-After guarantee; defining one is open work.

The Mu profile defines durable submission IDs and the seven-day minimum receipt
window separately from Micro-Id. Those semantics must not be generalized to every
service operation.

## 9. Example

The following invokes Mu's agent-discovery contract, not a mandatory core service.
Content-Length is illustrative for the two-byte body, excluding any display newline.

```http
POST / HTTP/1.1
Host: example.net
Micro-Protocol: mucp/1
Micro-Service: assistant
Micro-Endpoint: Assistant.ListAgents
Micro-Id: 12345678-1234-1234-1234-123456789abc
Authorization: Bearer <scoped-token>
Content-Type: application/json
Content-Length: 2

{}
```

```http
HTTP/1.1 200 OK
Content-Type: application/json
Cache-Control: private, no-store
Micro-Protocol: mucp/1
Micro-Service: assistant
Micro-Endpoint: Assistant.ListAgents
Micro-Id: 12345678-1234-1234-1234-123456789abc

{"agents":[],"publicQuestions":false}
```

## References

Normative dependencies: [HTTP semantics, RFC 9110](https://www.rfc-editor.org/rfc/rfc9110),
[JSON, RFC 8259](https://www.rfc-editor.org/rfc/rfc8259),
[protobuf JSON mapping](https://protobuf.dev/programming-guides/json/),
[bearer tokens, RFC 6750](https://www.rfc-editor.org/rfc/rfc6750), and the
versioned schema of the selected operation contract. Timestamp and binary
representations follow the protobuf mapping's referenced standards. The core's
Security and Privacy Considerations apply to this binding.

Licensed under [Apache-2.0](../proto/LICENSE).
