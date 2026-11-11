# Mu TypeScript SDK

Async client for browsers and Node 22+. No runtime dependencies. Apache-2.0.
Package name `@micro/mu` is proposed; this source has not been published to npm.
Build from the repository with `client/web/node_modules/.bin/tsc -p client/typescript`,
then install the directory with `npm install ./client/typescript`.

```ts
import { Mu } from '@micro/mu';
const mu = new Mu({server: 'https://micro.mu', token: process.env.MU_TOKEN});
const answer = await mu.ask('What is on my calendar?', {agentId: 'micro'});
console.log(answer.text);
```

A configured runtime and account credential are required. Asking an agent needs
`read write api:agent`; it uses that agent's configured tools and may cost credits.
Use `agents()`, `threads()`, `read(thread)`, `home()`, `inbox()` and `services()`
for the corresponding composition interfaces. Page tokens remain explicit.
Home, Inbox and Services require their separate `api:` capabilities.

For recoverable work, save `mu.prepare(prompt)` before sending, in private storage:

```ts
const submission = mu.prepare('Summarise my appointments');
// Persist submission, runtime origin, creation time and local account identity here.
const operation = await mu.submit(submission);
// Persist operation.id before waiting. Resume with mu.status(operation.id).
const answer = await mu.wait(operation);
```

After an uncertain submission, call `submit` with the **same** saved submission,
origin and account. Never edit the request or replace its message ID for a retry.
Mu's replay window is seven days; callers must enforce that limit on stored receipts.
Errors expose `submission` and, when available, `operation` and `thread` for recovery.
Receipts contain prompts: treat them as private. Credentials are never stored by the SDK.
`ask` is a convenience; use prepare/submit when process-crash recovery matters.
A local wait timeout does not cancel server execution. An in-flight read remains
bounded by the separate request timeout. Polling backs off to 30 seconds.

`Client` is the generic MUCP transport; `Mu` adds Mu's contracts. For a direct
service use `mu.protocol.call('contacts', 'Contacts.List', {})` with the appropriate
service-scoped token. Assistant scopes do not grant service access. Only services
actually bound by the server are callable. No assistant execution lives in this SDK.

Bearer credentials can be strings or async callbacks for host-managed renewal.
Browser applications may supply an explicit same-origin session/CSRF adapter;
OAuth login, secure storage and renewal stay with the application. Web and desktop
use this package through their existing session adapter. No redirects or automatic
action retries are performed. Unknown structured content retains its fallback.
