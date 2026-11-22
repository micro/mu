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
