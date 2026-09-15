# Browser client

React and shadcn/ui primitives, styled with Tailwind. Go serves the compiled
assets from this package; no Node process is needed to run Mu.

```sh
cd web
npm ci
npm run build
cd ..
go build .
```

Commit `dist/` with source changes. CI rebuilds and checks that the embedded
assets match, so `go build` and releases also work without a JavaScript toolchain.
Vite splits page modules and fingerprints assets for immutable browser caching.

`src/components/ui/` owns the shared controls. `layout.tsx` owns navigation,
page headings, sections, status messages and pagination. Page components compose
these; they do not bring a stylesheet. `src/index.css` contains the neutral theme
and document content rules. Email content is sandboxed separately from the app.

The shell owns conversations, Inbox, Account, Agents, and Admin. First-party
application screens live in the top-level `apps/` directory and use the same
React/shadcn controls. `apps/catalog.json` is their navigation catalogue; it does
not declare runtime services. Services remain under `service/` and the API/SDK
reference is generated from their registered specs.

Go embeds initial identity and conversation state in the HTML. The guest landing
is prerendered at build time, so it is visible before JavaScript starts. Other
screens load their own view data. Browser calls use `/client/call/`, with strict
CSRF checks and the existing service dispatcher enforcing identity, scopes,
quotas, and writes. Private searches remain POST bodies.

Downloads, raw mail, WebSockets, ActivityPub, and the immersive video player
retain their dedicated handlers. User-created app documents still execute in
their existing sandbox; the app catalogue, editor, and version history use React.
Legacy HTML helpers remain for protocol-specific views and compatibility tests;
new application screens belong in `apps/`, not those helpers.

Browser checks in `test/react_browser_test.go` and `test/react_apps_test.go` use real Go handlers with isolated
accounts. Set `MU_PLAYWRIGHT_MODULE` and `MU_LAYOUT_BROWSER` to run them locally;
the layout CI job installs Chromium and runs both client and legacy service tests.
