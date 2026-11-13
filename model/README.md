# Model

A model composes an LLM with a foundation, instructions, knowledge and services.
An agent uses that composition for a purpose and owns its execution.

This package owns Mu's shared foundation and system-prompt composition. Both the
agent and background writing paths use it. `Default()` returns a fresh definition
with Reminder and Aslam identified as reference sources through the existing
Islam service.

Knowledge entries describe sources; they do not fetch data, inject documents
into system instructions or grant access. Services remain subject to runtime
and caller permissions. Personal memory and conversation history belong to a
run, not a shared model definition.

LLM selection and provider calls still live in `internal/ai` and the existing
agent integration. The `LLM` and `Services` fields describe composition; this
first step does not change selection or tool dispatch. There is no new execution
loop, network endpoint or public protocol contract.
