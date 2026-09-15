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

The migrated routes are Home, Inbox (list, reader, compose and settings), Account,
Profile, Billing, Admin Users and the Apps catalogue. They return the client for
HTML and data for `Accept: application/json`; mutations retain their existing
owned, authenticated handlers. Other standalone service pages still use their
existing renderer. Move a route's markup and related dead helpers out of Go when
migrating it, preserving its API, authorization and operations.

Browser checks in `test/react_browser_test.go` use real Go handlers with isolated
accounts. Set `MU_PLAYWRIGHT_MODULE` and `MU_LAYOUT_BROWSER` to run them locally;
the layout CI job installs Chromium and runs both client and legacy service tests.
