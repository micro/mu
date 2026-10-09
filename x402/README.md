# x402 host

`internal/server` checks `x402.IsHost` once and delegates the configured host to
`x402.Handler`. Unknown host routes return 404; they never enter the consumer
router. Keep host-specific behavior here.

- The root package owns landing, tools, pricing, login, account, service tokens,
  OAuth consent, the dark shell and routing.
- `billing` owns credits, allowances, subscriptions, Stripe, crypto top-ups,
  usage and the browser checkout script. Both hosts call this implementation.
- `gateway` owns API credential normalization, wallet authentication, payment
  challenges, quota enforcement and settlement middleware on both hosts.
- `payment` owns the x402 protocol and facilitator client.

Account identity and credential persistence remain in `internal/auth`; the tool
catalogue and dispatcher remain in `internal/api`. No duplicate stores or ledger
migration are involved. Existing ledger filenames and webhook routes are kept.

Browser flow: sign up or sign in with a username/password, verify your email,
add credits or choose a
subscription, create a service token, then use it as `Authorization: Bearer …` on
this host's `/mcp` or `/api/v1/` endpoint. Checkout returns to the originating host.
Password signup rules, captcha, invite enforcement, rate limiting and verification
email delivery live in `registration` and are shared by both hosts. Stripe event
setup targets the primary configured origin, while checkout/portal returns use
the originating host. A bare `X402_HOST` denotes its public HTTPS origin; use an
explicit `http://` URL for an HTTP development host.

The `x402_session` cookie is host-only. Explicit API credentials take precedence;
the consumer `session` cookie is ignored. Service tokens use the shared account
and service scopes, not separate balances or identities. Token creation here is
limited to service access and a 90-day lifetime. Account management requires a
browser session and mutations require CSRF validation.

Direct x402 wallet payments still work without signing in. They settle per call;
requests authenticated with a service token use the shared credit balance.
