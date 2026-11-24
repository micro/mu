# Mu Pay

Mu Pay lets an application sell private, timed sessions with its Mu agent.
The owner configures the price and duration in **Edit agent → Mu Pay**.
The application's existing agent token creates checkout, reads the booking,
starts the clock and calls the agent. Keep that token on the application server.

Payments go through this instance's existing Stripe account and fund the agent
owner's Mu usage. A £3 place grants **300 credits**; three places cost £9 and grant
900 credits. This is a fixed product rate, not a live exchange rate. USD purchases
retain the existing rate of one credit per cent. Model and tool usage is charged
to the owner's balance normally; buying an hour is not an unlimited model budget.
There are no connected seller accounts or cash payouts in this release.

## Configure

The operator sets `STRIPE_SECRET_KEY` and `STRIPE_WEBHOOK_SECRET`. Configure
`https://<runtime>/stripe/webhook` for `checkout.session.completed` and
`checkout.session.async_payment_succeeded`. Mu Pay offers card checkout only.
The webhook must work: returning from checkout alone never unlocks a session.

Edit the agent and enable Mu Pay. Set:

- Price per participant, in minor currency units: `300` for £3.
- Currency `GBP` and duration `60` minutes.
- An HTTPS app return URL, such as `https://malten.me/session`.
- An optional HTTPS payment notification URL on the app's backend.

Save the agent, then reopen its settings to copy the notification signing secret
if using callbacks. Keep this secret on the backend. The existing agent builder
is operator-only; integration tokens cannot edit payment policy, choose a price,
change the return/callback URL or choose who receives credits.

`MU_PAY_GBP_CREDITS_PER_POUND` optionally overrides the default `100` credits per
pound. Mu rounds each place's credit grant down to a whole credit. Checkout stores
the price, quantity, duration, credit grant, return URL and notification secret;
changes apply only to new bookings. Back up the runtime's data directory,
including `pay/sessions.json` and the existing billing ledger together.

## Application flow

These are product HTTP endpoints, using JSON with snake_case field names and
numeric Unix timestamps. Their types are defined in
[`proto/assistant/v1/pay.proto`](../proto/assistant/v1/pay.proto); the operations
are not yet registered in the MUCP dispatcher. Use your own runtime base URL.

All authenticated requests use `Authorization: Bearer $MU_TOKEN`. Reads need
`read` and `api:agent`; mutations also need `write`. Cookie-authenticated writes
require the normal CSRF token. Requests are scoped to the token's owning account
and the named agent. No Mu credential goes to Stripe or the participant's browser.

### Create checkout and booking together

```bash
curl "$MU_URL/agent/malten/pay" \
  -H "Authorization: Bearer $MU_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"reference":"booking_123","participants":3}'
```

Example response (IDs and timestamps are illustrative):

```json
{
  "id": "opaque-session-id",
  "agent": "opaque-agent-id",
  "reference": "booking_123",
  "participants": 3,
  "amount": 900,
  "currency": "gbp",
  "credits": 900,
  "minutes": 60,
  "status": "pending",
  "checkout_url": "https://micro.mu/pay/opaque-session-id",
  "created_at": 1795518000
}
```

Store the returned `id` against the app's booking **before** opening `checkout_url`.
The host buys all three places in one checkout; Mu records the quantity and Malten
controls who occupies them. `reference` must be an opaque 1–128 character string
containing letters, digits, `_` or `-`. Participant count is 1–100.

Retry an ambiguous checkout request with the same reference and participant
count. Mu reuses the same purchase and Stripe idempotency key, including after a
restart. A changed participant count is rejected. Pending checkouts expire after
one hour; use a new booking reference after expiry. Retrying an old booking never
creates an extra purchase. A reference is scoped to owner and agent, not a token,
so rotating the token does not lose recovery.

The public `/pay/<id>` URL redirects to hosted Stripe checkout. On completion it
shows a return link to your configured app URL with `session=<id>` appended.
Cancellation also returns there. Treat that query parameter only as a lookup
hint; authenticate the app user and check its association with the booking.
Never treat the browser return, a supplied booking reference or `checkout_url`
as proof of payment.

### Recover payment and start

```bash
curl "$MU_URL/agent/malten/pay/$SESSION" \
  -H "Authorization: Bearer $MU_TOKEN"

curl -X POST "$MU_URL/agent/malten/pay/$SESSION/start" \
  -H "Authorization: Bearer $MU_TOKEN"
```

The read returns the same session resource. After the verified Stripe webhook
settles, `status` becomes `paid` and `paid_at` is present. Credits are granted once,
even if webhook deliveries repeat or the process stops between crediting and
saving the receipt. A guest's Stripe customer is never assigned to the owner.

Show **Start session** to the authenticated host once paid. Start changes status
to `active` and records `started_at` and `ends_at`. Repeated Start, late arrival,
reconnect and server restart keep the same end time. A pending session cannot
start. An expired session cannot restart; Start returns its expired state.

Use the signed callback below to update the booking without background polling.
Read the session on return to the app or when recovering a booking. If payment
is still pending, show that state and offer a manual refresh. A failed Stripe
webhook returns an error so Stripe retries; monitor webhook failures in Stripe.

### Talk during the session

```bash
curl "$MU_URL/agent/malten" \
  -H "Authorization: Bearer $MU_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"session":"opaque-session-id","prompt":"Let us begin."}'
```

The existing answer format returns `text` and `thread`. Once the first turn
binds the session to a thread, later calls with just `session` resume that thread.
You may supply `thread` explicitly; a different thread is rejected. Read the
session to recover the bound thread. A later **new paid session** can continue an
existing thread by supplying it on that new session's first prompt.

Mu checks active status before admitting each prompt. Requests admitted before
expiry may finish after it. Unpaid or expired requests return HTTP 402 without
calling the model. Omitting `session` does not let a paid thread continue as an
ordinary chat, including through the web streaming path.

The application authenticates participants, limits access to the purchased
number of places, admits only the host to Start, and shares the same `ends_at`
with everyone. A Mu agent token represents the owner, not individual participants.
The session is a time entitlement, not isolation from the agent's configured
knowledge, memory or services. Scope the agent's tools for its intended audience.

Normal private **Ask Malten** calls use neither the paid session nor its thread.
Public `@malten` requests continue using `public_only: true`. Neither requires a
booking. A paid call cannot also set `public_only`.

## Payment notifications

Mu posts a receipt to the configured URL after crediting the owner:

```json
{
  "id": "opaque-session-id",
  "payment": "cs_stripe_checkout_id",
  "reference": "booking_123",
  "agent": "opaque-agent-id",
  "participants": 3,
  "amount": 900,
  "currency": "gbp",
  "credits": 900,
  "paid_at": 1795518060
}
```

The header is `Mu-Pay-Signature: t=<unix-seconds>,v1=<hex-hmac>`.
Verify HMAC-SHA256 over the exact bytes of `<timestamp>.<raw-request-body>` using
the displayed signing secret as a UTF-8 string (do not hex-decode it). Compare in
constant time and reject timestamps outside a five-minute window. Parse JSON
only after verification, then match `id`, `reference`, `agent` and participant
quantity to the stored booking. Keep server clocks synchronized.

Persist the receipt and grant the booked places atomically before returning a
2xx response. Deduplicate by `id` (and keep `payment` for support). Every retry
has a fresh timestamp/signature but the same purchase. Failed deliveries retry
with backoff and resume after restart. Redirects are not followed; callbacks
cannot target private network addresses. Authenticated GET provides recovery if
your application missed a receipt. Notifications confirm payment, not Start.

## Testing and release checks

Automated tests cover the £9/900-credit checkout, policy snapshots, retry keys,
signed Stripe confirmation, duplicate settlement, recovery after the credit
commits but before the receipt saves, guest-customer separation, ownership and
scope checks, normal chat alongside paid calls, one-thread binding, repeated
Start, persisted expiry, callback signatures and notification retries.

Before enabling live payments, run the same flow against a staging runtime with
Stripe **test-mode** credentials and its test webhook secret:

1. Configure the agent for £3/60 minutes; create three places and follow the link.
2. Complete Checkout with [Stripe's test card](https://docs.stripe.com/testing) `4242 4242 4242 4242` and a future
   expiry. Verify `paid`, £9, three places and exactly 900 new owner credits.
3. Resend the event from Stripe; the balance and booking must remain unchanged.
4. Close the browser and restart Mu. Retrieve the booking, then Start twice;
   check that both responses have the same end time.
5. Make paid prompts with the same session, reconnect and confirm the same thread.
   Use a separate short-duration test booking to exercise actual expiry.
6. Check failed/cancelled checkout, a token from another account, public mentions
   and a normal private Ask Malten conversation.
7. Return 500 once from the app callback, then acknowledge its retry. Verify that
   places are not granted twice.

The automated Stripe/model tests use mocks; they do not prove live webhook
configuration or move money. Refunds, disputes, cash payouts and booking management
screens are not implemented by Mu Pay in this release. Stripe refunds do not
automatically revoke session access or reverse Mu credits; handle those cases
operationally before issuing a refund.
