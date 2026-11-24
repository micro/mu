# Embed a Mu agent

Call `POST /agent/<name>` from your application's backend with a bearer token
authorised for Agents. Keep the token on your server. `POST /agent` selects the
default agent.

```bash
curl "$MU_URL/agent/malten" \
  -H "Authorization: Bearer $MU_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"What would help here?","context":{"messages":[{"author":"Asim","text":"I am finding it hard to be patient"}]}}'
```

The response includes `text`, `thread` and `agent`, with optional typed `results`.
Persist the returned Mu `thread` against your app's stream and include it on later
requests. An unknown or foreign thread returns 404; an app's own stream ID cannot
be used to create a Mu thread. Omitting `thread` starts a new conversation.

## Surrounding messages

`context.messages` is an optional array ordered oldest first. Each entry contains
required, non-blank `text` and an optional `author` display label. Exclude the
current question: it belongs in `prompt`. The same field works on first requests
and follow-ups and supplements, rather than replaces, Mu's conversation history.

At most 24 entries and 16,000 UTF-8 bytes combined across all text and author
values are accepted. Invalid messages return HTTP 400; they are not silently
truncated. The existing request body limit is 32,000 bytes, including JSON
encoding and the prompt.

Messages are quoted, untrusted background, not system instructions. Author labels
do not authenticate participants or grant permissions. The calling application
must select only messages it is entitled to share.

For these synchronous API calls, excerpts are sent to the model for this turn;
Mu does not automatically copy them into its conversation or workflow records,
or extract durable memory from them. The saved answer may quote or refer to them.
Supply relevant excerpts again on later requests. Model-provider retention rules
still apply. A Mu thread is not an isolation boundary for the account's existing
memory or connected services.

`context.messages` does not change the agent's configured role, tools or access.
This endpoint does not implement the OpenAI Chat Completions request format.

## Agent addresses and public profiles

URL names are reserved across the server. Two agents can share a display name,
but their URLs differ (for example, `/agent/research` and `/agent/research2`).
Built-in agent names and route names such as `micro`, `code` and `new` are reserved.
Changing a display name does not change its URL. Deleted names stay reserved so
old links cannot be taken over by a different agent.

Existing agents are assigned names from their current tags, oldest record first
with ID breaking ties. Existing unique names are protected before suffixes are
assigned to collisions. Internal agent IDs, conversation records and email tags
are unchanged. Ambiguous old names and former email tags are not account-specific
URL aliases; integrations affected by a collision must use the assigned URL.
Owner-scoped opaque agent IDs remain accepted for compatibility.

In Edit agent, enable **Public profile** to publish the agent's name and
description at that URL. An optional Website links visitors to the external app;
when Mu Pay is enabled the profile also shows its price, duration and booking
link. Profiles are private by default, including for existing
agents. Publishing does not expose the system prompt, tools, memory, credentials
or conversations, and does not grant permission to run the agent. The owner
continues to see their chat at the same address. Other signed-in visitors see
the public profile, not a different agent with the same name. Unpublishing
removes the public page; the reserved name remains.

## Paid sessions

[Mu Pay](PAY.md) adds owner-configured checkout and timed private sessions using
the same agent token. Pass `session` on paid prompts; the session retains its
thread after the first turn. Ordinary private and public requests remain
available without a booking.
