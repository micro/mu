# Clients

Clients are focused interfaces to Mu's agents. They do not run a second agent
loop, own service permissions, or bypass account billing. The web app remains
the broader overview and management interface.

## Android

[Mu for Android](android/README.md) is a native voice and text client.
Sign in to an existing account, choose an agent, and talk to it.

## Where the existing code lives

| Interface | Client / adapter | Runtime owner |
| --- | --- | --- |
| Android | `client/android` | `agent/api.go` via HTTPS |
| CLI | `internal/cli`, including `ask.go` | `agent/api.go` via HTTPS |
| Web / PWA | `internal/app/html`, product page renderers | `agent`, `home`, `inbox` |
| Email / IMAP | External mail applications | `service/mail`; agent replies in `agent/mail` |
| XMPP | External XMPP applications | `service/chat`; agent replies in `agent/chat` |
| SMS / WhatsApp | Phone / WhatsApp applications | `service/sms`; agent replies in `agent/sms` |
| Client setup / addresses | `account/clients.go`, `internal/client` | Configuration and discovery, not an executable client |

New separately built clients belong here. Existing server protocol handlers stay
with their service; receiving an XMPP or SMTP connection does not make that
server a mobile client. The CLI is still part of the Mu binary and remains under
`internal/cli` until it is deliberately separated.

## Shared contract

`POST /agent/<name>`, `Accept: application/json`, bearer authentication.
Send `{"prompt":"…","thread":"…"}`; receive `{"text":"…","thread":"…","agent":"…"}`.
Responses may also include `results`: the runtime's typed result objects from
successful tool calls. Render known kinds as data, never execute model-authored
HTML or infer playback commands from prose. Voice clients may set
`context: {"voice": true}` for concise spoken framing; this grants no extra tools.

Omit the thread on the first message. The runtime owns conversation history,
tool access, account isolation and credit charging. Clients never retry
unconfirmed actions automatically.

OAuth uses browser authorization, S256 PKCE and explicit consent for
`read write api:agent`. No model keys or account passwords belong in clients.
