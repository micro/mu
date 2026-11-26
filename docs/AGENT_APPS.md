# Agent apps

An agent can have its own installable web app: a name, description, icon and
single-agent chat. Customers open a link and install it from their browser.
No APK build or app store submission is needed.

## Publish

1. Create and save an agent, then open **Edit agent → Configure app**.
2. Set the app name and description. Upload a PNG or JPEG icon under 2 MB;
   Mu generates padded 192px and 512px icons.
3. Choose free customer access or paid private sessions. For paid access,
   first enable Mu Pay and set the price and duration in agent settings. Leave
   the return URL empty to use the hosted app.
4. Enable **Publish app**, save, and share the displayed install link.

The app is available at `/agent/NAME/app/`. The public agent profile offers
**Open app**, and **Book a private session** when paid access is enabled.
The creator can always open that app URL; it does not redirect into the console.
Unpublishing rejects new app requests, including chat and payment creation.
It does not erase existing conversations or refund earlier purchases.

## Customer experience

Customers sign in using their own Mu account. Each has separate chat history;
the app contains no agent picker and no creator API token. Account management
and authentication use the existing Mu pages, returning to the app after login.

Free means free for the customer: the creator funds agent usage. Paid access uses
Mu Pay: the customer pays, returns to the app, and presses **Check payment** if
the payment has not yet been confirmed. **Start session** starts the clock.
Repeating Start does not extend it. After expiry, another payment is required.
A session continues its bound conversation; clicking New chat during that period
cannot use the same payment to open another paid conversation.

Payments fund the creator's existing Mu credit balance; they are not cash payouts.
The configured rate remains £3 for 300 credits. The hosted app sets its own
checkout return address and does not send the external integration's notification
callback. Customers cannot choose the credited owner, price or participant count.
The server binds each checkout to the signed-in customer before calling Stripe.
A receipt ID, redirect or install link is never proof of payment or ownership.

## Execution and privacy

Agent owns this customer boundary. Services remain lower-layer capabilities.
The app calls the existing agent loop with the selected agent's instructions and
model, intersecting its tools with the runtime's public tool catalogue. Tools
receive no creator or customer account identity. Private mail, files, memory,
connected accounts, account provider OAuth and management tools are unavailable.
Use an API-backed model for published apps.

Conversation records belong to the customer in `internal/thread`, with the
existing thread identifier. Only that customer's matching app threads can be
read or continued. Model usage is charged to the creator. The first version does
not grant published agents access to customer personal services or the creator's
private knowledge stores.

All browser mutations require a session and strict CSRF verification. Bearer
credentials are accepted only on the explicit customer API below, with read/write
and `api:agent` permissions. Creator settings and browser checkout reject tokens.
Shared configuration and prompt
contracts are defined in `proto/assistant/v1/app.proto`.

## Installation and offline behaviour

Each app has its own manifest ID, start URL and scope, plus a worker scoped to
its `/agent/NAME/app/` directory. Mu's original `/` identity and `/home` start
URL are unchanged. Names and icons can change without changing the app identity.
The apps share Mu's origin and sign-in: they are not separate browser security
origins. Separate domains remain future work. Native Android builds can use the same
published app; see the Android client README and customer API below.

Install is shown only when the browser offers an install prompt. Other browsers
can use Add to Home Screen; iPhone/iPad instructions point to Safari's Share menu.
Browser installation support varies, so multiple installed apps must be checked
on target devices before promising identical behaviour everywhere.

The worker caches only a public offline notice and shared static assets. It never
caches private chat, account or payment responses. Chat needs a connection;
failed sends retain the draft. There is no background account/payment polling and
no automatic replay of prompts or payment actions.

## Native Android and customer API

`client/android/README.md` describes building a named, signed Android app with
its own icon and package ID, pinned to one server and reserved agent name.
It renders chat natively and uses customer OAuth, not a WebView or creator key.
The first version can start and resume existing customer-owned paid sessions;
it does not implement in-app purchasing or automated store publication.

Authenticated routes beneath `/agent/NAME/app/`:

| Method/path | Request | Response |
|---|---|---|
| GET `profile` | — | `AppProfile` (public app fields only) |
| POST `threads` | `ListThreadsRequest` | `ListThreadsResponse` |
| POST `history` | `ReadThreadRequest` | `ReadThreadResponse` |
| POST `chat` | `AppPrompt` | `AppAnswer` |
| GET `sessions` | — | `AppSessions` (paid/active customer purchases) |
| GET `session/ID` | — | `PaidSession` |
| POST `session/ID` | empty object | `PaidSession` (start/resume) |

Types live in `proto/assistant/v1/{app,assistant,pay}.proto`. Responses from the
new routes use protobuf JSON; existing chat/session responses use snake_case,
which protobuf JSON clients also accept. Threads/history accept protobuf JSON
field names or snake_case. Chat accepts `prompt`, `thread`, `session` only.
Read-only tokens can POST history/list requests; chat and session start need
write permission. Cookie requests still require strict CSRF on every POST.
An explicit bearer token determines identity even if a browser cookie is present.

Lists and transcripts exclude personal Mu chats and other agents' app chats.
Pagination tokens are scoped to the customer and agent. Only receipts bound to
that customer at checkout are usable; external creator-created receipts cannot
be adopted. Opening or listing sessions never starts their timer. Requests are
on demand, with no background account polling or automatic prompt replay.
