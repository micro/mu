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

The current HTTPS interface below remains supported. The proposed MUCP assistant
contract is documented in [MUCP](../docs/MUCP.md), with its initial schema in
[proto/](../proto/README.md); clients do not use that binding yet.

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

## Distribution

The public landing opens the assistant or links to `/download`, with self-hosting
kept separate. `/download` explains browser/Home Screen installation and Android.
Android availability is refreshed in the background from the latest stable GitHub
release; only `mu-android.apk` is offered, never unsigned or debug APKs. The existing
release workflow requires the `ANDROID_KEYSTORE_BASE64`, `ANDROID_STORE_PASSWORD`,
`ANDROID_KEY_ALIAS` and `ANDROID_KEY_PASSWORD` secrets to produce that signed asset.
Keep the same signing key for updates. Do not label runtime Linux/macOS binaries
as desktop clients. The PWA starts at `/assistant`: agent selection, personal chats, shared tool-result
rendering, and optional tap-to-dictate/read-reply controls where the browser supports
them. Dictation is reviewed before sending and may use the browser
provider’s speech service. It does not offer background wake words. Account
and billing changes and agent creation open the runtime console. The console
remains at `/home`; installing the PWA does not remove it.
