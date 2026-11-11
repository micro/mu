<!-- SPDX-License-Identifier: Apache-2.0 -->

# Testing MUCP implementations

The test is whether an independent client can discover a runtime, send a request
to an agent and recover its answer. It must not need an Android or web interface,
Mu implementation imports, or knowledge of the runtime's storage.

Passing a smoke test is not full conformance. This document separates wire,
runtime, client and deployment evidence. RPC names remain unchanged while the
public contract is reviewed.

## One client, three environments

[probe.py](https://github.com/micro/mu/blob/main/proto/conformance/probe.py) uses
only Python's standard library and the published JSON/HTTP contract. It validates
response routing/correlation, uses TLS verification, never follows redirects,
and reports checks without printing account contents or credentials.

| Environment | What it establishes |
| --- | --- |
| CI HTTP server | The independent client can use Mu's real routing, authentication and JSON parser |
| CI assistant runtime | Real Submit, persistent queue and result recovery work together; only the model answer is deterministic |
| Deployed instance | The published origin, TLS, reverse proxy and deployed protocol are reachable and agree with the client |

The same executable runs in all three. Current CI uses Mu's real handlers in a
disposable test process; it does not yet launch the release binary and kill/restart
it. A separate unit peer tests that the probe rejects wrong correlation, does not
follow redirects, and preserves its saved submission identity across invocations.
It is not substituted for the Mu interoperability tests.

## SDK implementations

The reusable [TypeScript](../client/typescript) and [Python](../client/python)
SDKs are tested separately from the independent probe. The real assistant test
runs each SDK against the HTTP handler and durable queue, submits the same receipt
twice, recovers the result and checks that each request executed once. Shared JSON
fixtures also exercise the client transports. Only the model answer is replaced
in this test; it does not establish production or release-binary restart behaviour.

## Public and read-only checks

Run public discovery, including against the existing instance:

```sh
python3 proto/conformance/probe.py https://micro.mu
```

This checks availability and the primary contract, not agent execution. Add
`--wire` for a bounded set of malformed/read-only requests from
[wire.json](https://github.com/micro/mu/blob/main/proto/conformance/wire.json).
Without a credential, authenticated cases are explicitly reported as not run.
These cases currently target Mu's Assistant.ListAgents operation; they are a
Mu-profile test suite, not a claim that every MUCP server must offer that endpoint.

For agent/service discovery, provide `MU_TOKEN` through the test environment and
run `--reads`. Required Mu scopes are `read api:agent api:services`. No conversation
contents are printed or changed. `--wire` with that token also checks authenticated
parsing and the inclusive request-size boundary. Do not put tokens in arguments,
URLs or committed test files. There is no scheduled production polling job.

## Agent-to-agent execution

Use a dedicated test account with an agent whose tools are disabled. The probe's
prompt requests no tools, but a prompt is not a permission boundary. Grant
`read write api:agent`, configure a small credit budget, and supply its token via
`MU_TOKEN`. Execution may spend credits and creates a retained test conversation.

```sh
python3 proto/conformance/probe.py https://micro.mu \
  --test-account --agent-id TEST_AGENT_ID --delegate receipt.json
```

The client saves a stable submission ID with private file permissions before
sending. It deliberately discards the first acceptance body, recovers by submitting
the same ID/body, verifies repeated submissions identify the same operation and
thread, and polls with increasing delays (2–30 seconds) to a local deadline.
It checks a successful result structurally; it does not demand deterministic model
wording. Failed/interrupted work is reported and is never replaced with a fresh
submission. A timeout is not cancellation.

After stopping the client, resume with the same account credential and origin:

```sh
python3 proto/conformance/probe.py https://micro.mu \
  --test-account --resume receipt.json
```

The receipt contains no credential. Never switch principals when resuming: IDs are
caller-scoped. The probe refuses a different origin, an overwritten receipt or
replay after seven days. Preserve the receipt when a call fails ambiguously.

CI performs this sequence against Mu's real assistant handler with a deterministic
answer at the model boundary. A second client process resumes the saved receipt,
and the test asserts the runtime executed the request exactly once. This tests
that particular recovery case; it does not promise exactly-once external effects.

## Coverage matrix

| Requirement | Evidence | Remaining limit |
| --- | --- | --- |
| HTTP-01 endpoint/transport | Public live probe; Python origin/redirect tests; Go root-routing tests | TLS/proxy faults need deployment tests |
| HTTP-02 metadata | Shared wire cases for versions, IDs, missing/duplicate fields, routing and media type | Not every HTTP parser/proxy combination |
| HTTP-03 encoding | Wire cases for invalid/duplicate/unknown fields, multiple roots, size boundary; shared protobuf fixtures | More Unicode/numeric edge cases needed |
| HTTP-04 discovery | Same independent probe in CI and against live origin | Generic multi-contract/version negotiation not defined |
| HTTP-05 responses | Probe checks echoed routing/correlation, media type and object; wrong-correlation peer test | Broader malformed-response corpus needed |
| HTTP-06 authorization | TestMUCPRootAcceptsScopedToken, TestMUCPBoundary, TestCompositionIsolation, TestMUCPBrowserSessionRequiresCSRFForWrites | Live OAuth/revocation not exercised by token-based probe |
| HTTP-07 errors | Fixture status/code/body checks; existing authorization tests | Mu authorization gate retains legacy bodies |
| HTTP-08 uncertain outcome | TestMUCPIndependentDelegation, TestDurableOperationRecovery, TestMUCPDisconnectKeepsAcceptedWork | No release-binary crash or real external-side-effect test |
| Pagination ownership | TestMUCPPaginationIsolation, TestPagesBindCallerAndOperation | Live pagination under concurrent changes needs more coverage |

Commands used by CI:

```sh
python3 proto/conformance/test_probe.py
go test ./internal/server -run '^TestMUCPIndependentProbe$' -count=1
go test ./agent -run '^TestMUCPIndependentDelegation$' -count=1
go test ./agent ./internal/mucp ./internal/server -short
```

The normal backend job runs the Go tests as part of `go test ./... -short`.
The protocol job runs the Python peer tests and schema/fixture checks. Python is
required in CI; a local environment lacking it reports a skipped Go probe test.
Tests and fixtures are versioned with the specification. New bindings need their
own equivalent wire suite, not exemptions based on transport/library behaviour.

## Next deployment tests

On a disposable instance of the release binary, test process death after acceptance,
queued and in-flight recovery, concurrent identical submissions, credential
revocation during polling, and a proxy that drops responses or returns 502/504.
Use two test principals to check foreign thread/receipt denial. Count execution
at a controlled tool boundary as well as counting returned receipts.

On production, run the small public/read suite and opt-in dedicated-account task.
Do not restart production or deliberately revoke a person's active credentials.
A further independently written server is needed to validate that the specification
is sufficient for implementations other than Mu. Go-micro implementation work
must run these tests without importing Mu product packages.

## Known specification work

The documents now separate core, binding and Mu profile, but are still a working
draft. Open items include general contract discovery/version selection, namespace
and extension policy, uniform authorization errors, a portable OAuth bootstrap
profile, full operation transition/concurrency guarantees, negotiated polling
limits and an immutable specification release. Do not claim IETF status, federation
or full conformance from the current suite.

Licensed under [Apache-2.0](../proto/LICENSE).
