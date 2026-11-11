# Mu

An open runtime for personal assistants.

## Overview

Mu brings together agents, connected services, accounts, permissions and persistent
history. Run it yourself or use a hosted instance, then access your assistants
through a focused client or the broader web console.

**Micro** is the general-purpose assistant. Other agents can have their own
instructions and permitted tools. The Android app and standalone web client provide voice
and text interaction; the web console provides Home, Inbox, Agents and Services
for overview, configuration and direct access to capabilities.

Services supply mail, calendar, contacts, information, media and storage. Agents
use those capabilities to carry out requests. Mu runs them together as one Go
binary, using Go Micro as its underlying framework.

## Architecture 

```text
  Android / web / other clients
              |
       assistant contract
              |
   MUCP over HTTPS + JSON (*)
              |
      Mu runtime (server)
      +----------------------+
      | Authenticated entry  |
      |          |           |
      | Agent + thread       |
      |          |           |
      | Permitted tools      |
      |          |           |
      | Services             |
      +----------------------+
```

(*) MUCP uses readable JSON over HTTPS at `/`. Proto defines shared Go/Java
message types; binary RPC and generated browser clients are not required.

See [Architecture](docs/ARCHITECTURE.md) for the layers and assistant interaction,
[MUCP](docs/MUCP.md) for the protocol and its specifications, and
[contracts](proto/README.md) for the assistant interface.

## Purpose

Mu is intended to provide a deliberate way to use connected services without an
advertising-driven or engagement-driven experience. You choose the agents and
connections. Self-hosting gives you control of the runtime; model providers and
connected services still receive the data needed for the operations you enable.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/micro/mu/main/install.sh | sh
~/.local/bin/mu --serve --address 127.0.0.1:8080
```

See [Install Mu](docs/INSTALL.md) for the quick start.

Optional [documentation](docs/README.md) covers hosting, configuration, mail and development.

## Clients

Use Mu through the API, CLI, web or mobile apps. See the [client map](client/README.md)
for implementation details.

### API

```bash
curl "$MU_URL/agent" \
  -H "Authorization: Bearer $MU_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"What needs my attention?"}'
```

The response includes `text` and `thread`; send the thread identifier
back to continue. Requests and credentials belong in request bodies and headers.
The [Mu runtime contracts](docs/MUCP-RUNTIME.md) provide Home, Inbox, agent and service discovery
interfaces, with durable personal requests and typed results. Mu is its first
runtime implementation. [Service contracts](proto/service) begin with Contacts;
other services remain available through agent tools.
A separately configured x402 host provides paid service calls.

### CLI

The binary acts as a client. It defaults to the hosted instance; set
`MU_URL` or use `mu login https://your.host` for your own server.

```bash
mu ask "What needs my attention?"
mu inbox list
mu help
```

The standalone [Mu CLI](client/cli/README.md) connects to a remote runtime with
`mu-cli login`, `mu-cli ask` and `mu-cli chat`, without installing the server.

### Web

Open **http://localhost:8080** to create your account and choose an AI provider.

The full web experience at `/home` installs as the **Mu PWA**.
The separate [React/TypeScript client](client/web/README.md) runs independently
with `npm run dev` or from the `mu-web.zip` release using `node serve.mjs`.
Mu also serves its production build at `/app/`, reusing your website sign-in.
[Desktop apps](client/desktop/README.md) package this client for Windows, Mac and Linux.

### Mobile

The [Android app](client/android/README.md) provides voice and text interaction
with your agents. It connects to micro.mu by default; choose your own server in Settings.
Use the web client on iPhone and iPad, or install the runtime PWA for the full web experience.
Find available apps on the [downloads page](https://micro.mu/download).

## Commercial

Mu’s runtime and client implementations are available under the [GNU AGPL v3](LICENSE),
which permits personal and commercial use, including charging for a hosted service. No commercial licence
is required when you comply with the AGPL.

The AGPL includes source-sharing obligations:

- When distributing Mu or a covered derivative, provide the corresponding source
  to recipients as required by the licence.
- When running a modified version over a network, prominently offer its
  corresponding source to the users interacting with it.

For a proprietary product based on Mu, a paid commercial licence can provide
alternative terms for keeping covered modifications closed—for example, a
customised assistant runtime or a branded client. Visit
[Micro on GitHub Sponsors](https://github.com/sponsors/micro) for sponsorship and
commercial licensing enquiries. Scope and fees are agreed
separately; third-party components retain their own licences.

An independently written client connecting through MUCP or the API does not need
a commercial licence merely to connect. The
[protocol contracts and generated types](proto/README.md#licence), and the
[MUCP specification](docs/MUCP.md), are Apache-2.0 licensed so independent clients
and runtimes can reuse them, including in proprietary products. `proto/generate`
remains AGPL. See [LICENSING.md](LICENSING.md) for the directory-level terms.
This section summarises
the options; the [licence](LICENSE) or your commercial agreement governs use.
