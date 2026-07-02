# Developer / API portal (the second face)

Mu has two faces on one backend:

- **Consumer face** — the everyday agent at `/` (branded **Mu**, the product).
- **Developer / API face** — the same go-micro services presented for machine
  consumers: every capability as an [MCP](MCP.md) tool and REST endpoint, paid
  per call over [x402](https://x402.org). Branded from the domain it's served on.

This is a *face*, not a separate product — same binary, same services, no second
deployment. Every self-hosted instance has it. `m3o.com` is just a vanity domain
we point at our own instance's developer face; a self-hoster gets the identical
capability on their own domain with **no code changes**.

## How it's triggered (domain-agnostic by design)

Nothing about any specific domain is compiled into Mu. The face is selected two
ways, both generic (`internal/app.PortalMode`):

1. **`/developers` path** — always available on every instance, zero config.
   Visit `https://<your-host>/developers`.
2. **`X-Mu-Portal: developer` request header** — when a reverse proxy sets this,
   the portal is served at the domain's **root** (`/`). This is how you dedicate
   a whole domain (e.g. `m3o.com`) to the developer face. The domain → face
   mapping lives entirely in *your* proxy config, never in the codebase.

## Branding (derived from the domain)

The portal wordmark is derived from the request `Host` — the second-level label,
uppercased (`internal/app.PortalBrand`):

| Host | Wordmark |
|------|----------|
| `m3o.com` | `M3O` |
| `api.acme.dev` | `ACME` |
| `tools.foo.io` | `FOO` |

So any domain pointed at the instance brands itself; there's no per-domain
config and no image logo to swap. Common subdomain prefixes (`api.`, `dev.`,
`developers.`, `www.`) are stripped first.

Override the exact text with the **`X-Mu-Portal-Brand`** header (set by the
proxy) when you want specific casing or a multi-part TLD (`foo.co.uk`):

```
X-Mu-Portal-Brand: GoMicro
```

The consumer product keeps its own identity (**Mu**) regardless of host — only
the developer face auto-brands.

## nginx: dedicate a domain to the developer face

Point the domain at the same server as the consumer site, then:

```nginx
server {
  server_name m3o.com;                 # your vanity API domain
  location / {
    proxy_pass http://127.0.0.1:8081;  # the same Mu backend
    proxy_set_header Host $host;
    proxy_set_header X-Mu-Portal developer;      # <- serve the portal at /
    # proxy_set_header X-Mu-Portal-Brand M3O;    # optional: force the wordmark
  }
}
```

Your consumer domain's vhost sets no such header and behaves normally. Both
domains share one backend and one deploy.

## Self-host parity

A developer running Mu gets the consumer app, the MCP endpoint (`/mcp`), the
REST API (`/api`), **and** this developer portal (`/developers`) out of the box.
To give it a vanity domain they copy the one `X-Mu-Portal` line above with their
own domain — nothing in the binary knows or cares which domain it is.
