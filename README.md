# Mu

An open runtime for personal assistants.

Mu brings together agents, connected services, accounts, permissions and persistent
history. Run it yourself or use a hosted instance, then access your assistants
through a focused client or the broader web console.

**Micro** is the general-purpose assistant. Other agents can have their own
instructions and permitted tools. The Android app and focused PWA provide voice
and text interaction; the web console provides Home, Inbox, Agents and Services
for overview, configuration and direct access to capabilities.

Services supply mail, calendar, contacts, information, media and storage. Agents
use those capabilities to carry out requests. Mu runs them together as one Go
binary, using Go Micro as its underlying framework.

## Client, protocol and runtime

```text
  Android / PWA / other clients
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
[MUCP](docs/MUCP.md) for the wire binding and recovery rules, and
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

See [Install Micro](docs/INSTALL.md) for the quick start. 

Optional [documentation](docs/README.md) covers hosting, configuration, mail and development.

## Web

Open **http://localhost:8080** to create your account and choose an AI provider.

The focused assistant at `/assistant` can be installed as a PWA. The broader
runtime console remains at `/home`.

## Clients

Native and protocol clients provide direct agent use. See the [client map](client/README.md)
and [Android voice and text client](client/android/README.md).

## CLI

The binary acts as a client. It defaults to the hosted instance; set
`MU_URL` or use `mu login https://your.host` for your own server.

```bash
mu ask "What needs my attention?"
mu inbox list
mu help
```

## API

```bash
curl "$MU_URL/agent" \
  -H "Authorization: Bearer $MU_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"What needs my attention?"}'
```

The response includes `text` and `thread`; send the thread identifier
back to continue. Requests and credentials belong in request bodies and headers.
A separately configured x402 host provides paid service calls.

## Commercial

If you want to run your own for commercial use please reach out for licensing terms and conditions. Fees are applicable.
