# Listing this server

Where to publish a Mu instance so agents and their people can find it, and what
each registry asks for. Written for the operator of an instance — micro.mu or
your own.

Everything here needs a person with an account somewhere, so none of it can be
automated from inside Mu. What Mu does provide is the material each one asks
for: `server.json` in the repository root, the metadata at
`/.well-known/oauth-protected-resource`, and the tool catalogue at `/tools`.

## What a registry checks

The submission forms converge on the same questions. Mu answers all of them:

| Asked for | Where it comes from |
|---|---|
| Transport must be Streamable HTTP | `/mcp`, served by go-micro's MCP gateway |
| Auth flow, callback registered | OAuth 2.1 with dynamic client registration; discovery at `/.well-known/oauth-authorization-server` |
| Every tool has a human-readable title | Derived — `internal/api/annotations.go` |
| Annotations present, reads and writes separated | Derived from each endpoint's `Spec` and its verb |
| Tool names ≤ 64 characters | Enforced by `TestToolsListCarriesTitlesAndAnnotations` |
| Documentation URL | <https://micro.mu/docs> |
| Privacy policy URL | <https://micro.mu/privacy> |

Check the first four against a running instance before submitting anywhere:

```bash
curl -s -X POST https://micro.mu/mcp -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' |
  python3 -c 'import sys,json; ts=json.load(sys.stdin)["result"]["tools"]; \
    print(len(ts),"tools;", sum(1 for t in ts if t.get("title")),"titled;", \
    sum(1 for t in ts if t.get("annotations")),"annotated")'
```

## The official registry

`registry.modelcontextprotocol.io` is the one clients read, so it is the one
that matters. Publishing uses the `mcp-publisher` CLI:

```bash
mcp-publisher init          # reads server.json in this directory
mcp-publisher login dns --domain micro.mu   # or: login github
mcp-publisher publish
```

The namespace in `server.json` is `mu.micro/mu`, which is a DNS namespace: it
requires proving ownership of the domain with a TXT record rather than a GitHub
account. That is the right choice for a self-hostable server — the name should
belong to whoever runs the instance, not to whoever holds the repository.

A fork publishing its own instance should change `name`, `websiteUrl` and the
`remotes[0].url` to its own domain, and verify that domain. Publishing a fork
under `mu.micro` will fail verification, which is the intent.

## Anthropic's connectors directory

Submitted through the portal in Claude.ai settings, documented at
<https://claude.com/docs/connectors/building/submission>. It is the strictest
review and the highest-intent audience. Have ready: the server URL, a tagline,
two or three sentences of primary use cases, the documentation and privacy
policy URLs, an icon, and a test account.

The requirements it enumerates — Streamable HTTP, registered OAuth callback,
per-tool titles, annotations, read/write separation, names under 64 characters
— are the table above. They are all satisfied by a current instance.

## The rest

Smaller directories, worth an afternoon between them once the two above are
done:

- **Smithery** — `smithery mcp publish`, or the web dashboard
- **Glama** — <https://glama.ai/mcp/servers>
- **mcp.so** — the largest by volume
- **MCP.Directory** — pulls metadata from the GitHub repository automatically
- **PulseMCP**, **Cursor's directory** — submission forms
- **punkpeye/awesome-mcp-servers** — a pull request

This list moves quickly. Check a directory still exists and still accepts
submissions before spending time on it.

## What to say

The catalogue is the argument. Most entries in these directories are a wrapper
over one API; this one runs the mail server, the feed aggregator, the search
index and the app sandbox, and exposes 67 tools over a single endpoint. Lead
with that rather than the tool count — the count is a consequence.
