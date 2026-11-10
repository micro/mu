# Mu Web

The focused assistant client, served at `/app/` by Mu. Choose an agent, speak or
type, open personal chats, and view structured results. The full built-in web
experience remains at `/home`, with Inbox, Agents and Services.

The client uses MUCP's JSON binding at `/` for ListAgents, ListThreads,
ReadThread and Ask. It renders the data locally; it does not fetch runtime-rendered
chat HTML. Model output is never executable markup. Account identity, history,
permissions and billing remain the runtime's responsibility.

## Install

Open `/app/` and use the browser's Install or Add to Home Screen option. This
installs **Mu Web**, with its own manifest identity and service-worker scope.
The original **Mu** installation retains identity `/` and starts at `/home`.
Browsers decide when installed manifest metadata updates; no reinstall is required
to use either experience. `/assistant` is removed, not retained as an alias.

Browser speech is optional and explicitly activated. Dictation fills the composer
for review before sending. Speech recognition may use the browser vendor's online
service. There is no background wake word. YouTube remains a visible video player;
audio results with playable URLs have native audio controls.

## Build and host

```sh
python3 client/web/build.py /tmp/mu-web.zip
```

Releases include `mu-web.zip`. It contains the same static files embedded by Mu,
plus the canonical shared stylesheet and icons. Serve it at `/app/` over HTTPS.
For local development, loopback HTTP is sufficient. Do not open it as `file://`.

**Current authentication is same-origin:** use Mu's bundled serving, or put these
files and the runtime behind the same reverse proxy. Forward `/` MUCP requests and
runtime account/login routes to Mu. Static hosting on an unrelated origin is not
yet supported; it needs browser OAuth and an explicit runtime origin policy.
No credentials, transcripts or private replies are cached or stored by this client.
The service worker provides a separate installation scope, not offline chat.

This ZIP is a web deployment, not a desktop installer. Packaging this interface in
a desktop shell and supporting connections to independently selected runtimes are
subsequent work. Linux/macOS Mu binaries are runtime releases, not desktop clients.

## Development

Keep protocol and rendering behaviour in `static/client.js`. Shared design rules
remain in `internal/app/html/mu.css`; the release build copies that source rather
than maintaining a second stylesheet. `web.go` only embeds and serves static files.
The composition in `internal/server` mounts them and the shared assets.

Run `go test ./client/web -short`. With Chrome installed, set `CHROME_PATH` and run
`go test ./client/web -run TestFocusedClientBrowser`; `UI_SCREENSHOTS` selects the
screenshot output directory. CI exercises mobile and desktop sizes.
