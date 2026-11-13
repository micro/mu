# Model

A model composes an LLM with a foundation, instructions, knowledge and services.
An agent uses that composition for a purpose and owns its execution.

This package owns Mu's model composition, shared foundation, system-prompt
building, provider selection, inference, metering and health. Both the
agent and background writing paths use it. `Default()` returns a fresh definition
with Reminder and Aslam identified as reference sources through the existing
Islam service.

Knowledge entries describe sources; they do not fetch data, inject documents
into system instructions or grant access. Services remain subject to runtime
and caller permissions. Personal memory and conversation history belong to a
run, not a shared model definition.

The runtime consumes `LLM`, `Foundation`, `Instructions` and `Services`.
Agents accept a composition through `QueryOpts.Definition`; writing accepts one
through `Prompt.Definition`. Existing model IDs remain per-run overrides.
An explicit provider uses only that provider's configured credentials and fails
if it is unavailable. With no explicit provider, existing selection rules apply.

A nil service list inherits the runtime's available services. An explicitly
empty list disables tools. A nonempty list restricts the available services;
agent tool scopes, guest/group restrictions and account authorization still apply.

This package does not own storage. Agent records persist their standing prompt,
LLM ID and service scope; these are composed with the shared defaults at runtime.
Knowledge sources remain references retrieved through existing services, not
persisted retrieved content or instructions. No new model store is introduced.
