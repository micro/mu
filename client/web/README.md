# Mu web client

A standalone React + TypeScript client for Mu, built with Vite. It runs
independently, connects to a runtime you choose, and signs in through that
runtime's browser OAuth flow. It uses MUCP for agents, chats and replies.

The runtime's PWA is separate: install the full Home/Inbox/Agents/Services web
experience from your runtime. This client has no PWA manifest or service worker.
Mu also serves the client's production build at `/app/`. `/assistant` is removed.

## Run from source

With Node.js 24 or later, from this directory:

```sh
npm ci
npm run dev
```

Open the local address Vite prints. Enter `https://micro.mu` or your own runtime
address, sign in and approve access. No local Mu server, reverse proxy, model
keys or dashboard session is required. The runtime must support browser-client
OAuth/CORS and MUCP. Local runtimes may use HTTP on a loopback address.

## Run a release

Download `mu-web.zip` from [Mu releases](https://github.com/micro/mu/releases),
extract it, then run:

```sh
node serve.mjs
```

Open **http://127.0.0.1:5173**. The ZIP includes the compiled app, styles, fonts and
launcher; it needs Node.js, but no npm install or build. `PORT` and `HOST` configure
the launcher; it listens only on loopback by default. For public hosting, serve
`static/` over HTTPS with any static web server.

This is a runnable browser client, not an Electron/Tauri desktop installer.
Desktop packaging is separate work. Mu's Linux/macOS runtime binaries are not
client installers.

## Build and test

```sh
npm test
npm run build
npm start
python3 build.py /tmp/mu-web.zip
```

The production build in `static/` is committed so `go build` remains self-contained
and does not require Node. CI rebuilds and checks for drift. The build consumes
the canonical stylesheet in `internal/app/html/mu.css`; there is no separate theme.

`go test ./client/web -short` checks the embedded client boundary. With Chrome,
set `CHROME_PATH` and run `go test ./client/web -run TestFocusedClientBrowser`.
That test hosts client and runtime at different origins and exercises sign-in,
explicit bearer authentication, responsive rendering, chats and reply recovery.

## Accounts and capabilities

OAuth uses S256 PKCE, state verification, explicit consent and `read write api:agent`
permissions. Tokens and pending sign-in data stay in tab-scoped session storage;
only the selected runtime address is remembered in local storage. Disconnect
clears this client's session. Revoke access in the runtime account settings.
Client requests omit cookies. Cross-origin browser access is restricted to MUCP
and the OAuth registration/token endpoints; it does not expose dashboard routes.

The runtime owns history, tools, permissions, billing and model execution. Replies
and structured results are rendered as data, never executable model-authored HTML.
Unknown content uses its text fallback. Unconfirmed messages are never resent
automatically. Chats recover accepted replies after a connection failure.

Browser speech is optional and explicitly activated. Dictation is reviewed before
sending, and may use the browser vendor's speech service. There is no background
wake word. YouTube stays a visible player; playable audio results use audio controls.
