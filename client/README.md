# Clients

Clients are focused interfaces to Mu's agents. They do not run a second agent
loop, own service permissions, or bypass account billing. The web console remains the broader overview and management interface. Micro is
the general-purpose agent, not the name of every client or agent.

The target architecture is client + MUCP + Mu runtime. Proto supplies shared types;
the external binding uses human-readable JSON over HTTPS. A client need
not use generated code or a protobuf runtime. See the [architecture diagrams](../docs/ARCHITECTURE.md).

## Android

[Mu for Android](android/README.md) is a native voice and text client.
Sign in to an existing account, choose an agent, and talk to it.

## Where the existing code lives

| Interface | Client / adapter | Runtime owner |
| --- | --- | --- |
| TypeScript SDK | `client/typescript` | Generic MUCP transport and Mu composition |
| Python SDK | `client/python` | Generic MUCP transport and Mu composition |
| Android | `client/android` | `agent/mucp.go` via MUCP |
| Standalone CLI | `client/cli` (`mu-cli`) | `agent/mucp.go` via MUCP |
| Runtime/operator CLI | `internal/cli` (`mu`) | Local runtime; legacy remote commands via HTTPS |
| Desktop | `client/desktop`, bundled React app and Electron | `agent/mucp.go` via MUCP |
| Standalone web | `client/web` | `agent/mucp.go` via MUCP |
| Full web / PWA | `internal/app/html`, product page renderers | `agent`, `home`, `inbox` |
| Email / IMAP | External mail applications | `service/mail`; agent replies in `agent/mail` |
| XMPP | External XMPP applications | `service/chat`; agent replies in `agent/chat` |
| SMS / WhatsApp | Phone / WhatsApp applications | `service/sms`; agent replies in `agent/sms` |
| Client setup / addresses | `account/clients.go`, `internal/client` | Configuration and discovery, not an executable client |

New separately built clients belong here. Existing server protocol handlers stay
with their service; receiving an XMPP or SMTP connection does not make that
server a mobile client. Runtime/operator commands remain under `internal/cli`;
the standalone client is independently built under `client/cli`.

## CLI separation

The [standalone terminal client](cli/README.md) lives in `client/cli` and builds as `mu-cli`.
It uses browser OAuth for sign-in and MUCP for agent discovery, conversations and supported runtime
capabilities, without importing the server implementation. `mu` retains
runtime startup, configuration and local operator commands. Move remote-use
commands to `mu-cli` only after it provides equivalent behaviour and documented
migration instructions; the existing commands remain available until then.

## Shared contract

Android, desktop and [Mu Web](web/README.md) use MUCP: JSON over HTTPS at `/`, with
`Micro-Protocol: mucp/1` and the assistant operations in [proto](../proto/README.md).
See [MUCP](../docs/MUCP.md) for headers, authentication and error handling.
The runtime owns execution, history, permissions and billing. Clients render
structured content safely. Personal requests use durable Submit/ReadOperation;
public locked-phone questions retain the restricted stateless Ask mode. New OAuth
consent includes Home, Inbox and service discovery. Previously issued agent-only
tokens need reconnection for those views.

Web/desktop expose Home, Inbox, Agents and Services through the shared contract;
Android keeps its voice/chat focus and offers Home, Inbox and Services from its
account menu. The runtime PWA retains direct service pages and administration.
The existing operator CLI and channel bridges keep their current interfaces; the
[independent JSON example](../proto/examples/client.py) demonstrates MUCP without
Mu implementation dependencies.

## Distribution

Android releases provide the signed `mu-android.apk`. The React/TypeScript web client runs independently with `npm run dev`,
or from `mu-web.zip` using `node serve.mjs`. Mu also serves its build at `/app/`. The existing full-web Mu PWA starts at `/home` and keeps its original
installation identity. `/assistant` is removed.

The runtime PWA is the full Home/Inbox/Agents/Services experience. The web app
has no PWA manifest. Hosted `/app/` reuses the website session with CSRF protection;
independent apps use browser OAuth with S256 PKCE and scoped tokens. Server selection
lives in Settings, with micro.mu as the independent-app default.

[Desktop apps](desktop/README.md) package Mu Web with Electron for Windows, macOS
and Linux. Release CI produces installers; macOS consumer publication requires
Developer ID signing and notarization. The same web app remains available on
platforms without a native download. No app-store submission is required for desktop.
