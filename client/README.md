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
| Android | `client/android` | `agent/mucp.go` via MUCP |
| CLI | `internal/cli`, including `ask.go` | `agent/api.go` via HTTPS |
| Desktop | `client/desktop`, bundled React app and Electron | `agent/mucp.go` via MUCP |
| Standalone web | `client/web` | `agent/mucp.go` via MUCP |
| Full web / PWA | `internal/app/html`, product page renderers | `agent`, `home`, `inbox` |
| Email / IMAP | External mail applications | `service/mail`; agent replies in `agent/mail` |
| XMPP | External XMPP applications | `service/chat`; agent replies in `agent/chat` |
| SMS / WhatsApp | Phone / WhatsApp applications | `service/sms`; agent replies in `agent/sms` |
| Client setup / addresses | `account/clients.go`, `internal/client` | Configuration and discovery, not an executable client |

New separately built clients belong here. Existing server protocol handlers stay
with their service; receiving an XMPP or SMTP connection does not make that
server a mobile client. The CLI is still part of the Mu binary and remains under
`internal/cli` until it is deliberately separated.

## Shared contract

Android, desktop and [Mu Web](web/README.md) use MUCP: JSON over HTTPS at `/`, with
`Micro-Protocol: mucp/1` and the assistant operations in [proto](../proto/README.md).
See [MUCP](../docs/MUCP.md) for headers, authentication and error handling.
The runtime owns execution, history, permissions and billing. Clients render
structured content safely and never automatically resend an unconfirmed action.

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
