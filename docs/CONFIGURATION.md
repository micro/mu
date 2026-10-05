# Configuration

Optional providers, service keys and runtime settings. Start with the
[installation guide](INSTALL.md) if you have not run Micro yet.

## Configuration

Nothing is required to start. Each key below switches on the feature next to it,
and every one of them can also be set at `/admin/config` in the browser once you are
admin, so the environment is for the things you want fixed at deploy time.

```bash
# An AI provider — one of these, for the agent, chat and summaries
export ANTHROPIC_API_KEY="your-key"   # Claude, from console.anthropic.com
# export ATLASCLOUD_API_KEY="your-key" # Atlas Cloud (DeepSeek, Qwen), also images
# export GEMINI_API_KEY="your-key"     # Google Gemini
# export OPENROUTER_API_KEY="your-key" # OpenRouter (one key, many models)
# export OPENAI_BASE_URL="http://localhost:11434/v1"  # Ollama or any compatible endpoint
# export OPENAI_MODEL="llama3.2"                      # which model that endpoint serves

# Video
export YOUTUBE_API_KEY="your-key"  # Google Cloud Console

# Places — falls back to OpenStreetMap without it
export GOOGLE_API_KEY="your-key"   # enable Places API (New) and the Routes API

# Web search
export BRAVE_API_KEY="your-key"

# Card top-ups for credits
# export STRIPE_SECRET_KEY="sk_live_..."
# export STRIPE_WEBHOOK_SECRET="whsec_..."
```

Mu also reads a dotenv file at startup: `$MU_ENV_FILE`, then `~/.env`, then
`~/.mu/.env` — the first that exists wins.

Every setting the code reads is listed under Configuration reference below.

## Taking payments

Callers pay in credits, prepaid against an account. Set the `STRIPE_*` keys to
let people buy them by card; without those keys your instance runs with no
metering, which is usually what you want for one you run for yourself.

New personal signups receive 100 nontransferable usage credits once. Existing
accounts and purchased/legacy balances are unchanged. The default daily allowance
is 20 credits; an operator override in the data directory still takes precedence.
Signup credits remain until spent and follow daily and monthly credit in the
consumption order. Password and Google signup use the same grant.

Operation prices and the free daily allowance are in `quota.json`. Provider
estimates are under Admin → Traffic → Spend; product credits are not a provider
spending limit.

### Monthly subscriptions

Starter defaults to $12/month with 1,000 monthly usage credits; Pro defaults to
$45/month with 4,000 monthly usage credits when
`STRIPE_SECRET_KEY` and `STRIPE_WEBHOOK_SECRET` are configured. Override
`STARTER_CENTS` and `STARTER_CREDITS`, or `SUBSCRIPTION_CENTS` and
`SUBSCRIPTION_CREDITS`, together for different terms;
set the corresponding cents value to 0 to stop new subscriptions for that tier. Existing subscriptions
retain their purchased price and allowance. Monitor provider spend as usage grows.
A publishable key is not required for hosted Checkout. Account and Pricing use
these same settings. Stripe creates the recurring product/price during Checkout;
no manual catalogue setup is needed. Changed settings apply to new subscribers,
not existing contracts.

Free includes a weekly brief, Starter a daily brief, and Pro adds an optional
daily plan in the same delivery and one recurring research topic. All schedules
are opt-in in Events. Brief and plan use bounded context with no tool calls and
do not spend credits. Research uses a metered search and summary, with a visible
per-check credit limit; unchanged results skip the summary and delivery. Benefits
follow the paid period even if monthly credits are exhausted or renewal canceled.
Downgrades enforce weekly briefs and pause Pro research. No service surcharge is
added. Operator margins must cover processing and included generation costs.

Before the first subscription, Checkout finds this site's enabled
`https://<your domain>/stripe/webhook` endpoint and adds the events below,
preserving existing events and the signing secret. The API key needs webhook
endpoint read/write access. An absent or disabled endpoint stops Checkout before
payment. You can also configure these events yourself:

- `checkout.session.completed` and `checkout.session.async_payment_succeeded`
- `invoice.paid`, `invoice.payment_failed`, `invoice.payment_action_required`
- `customer.subscription.created`, `customer.subscription.updated`,
  `customer.subscription.deleted`

Requests retrieve canonical objects with Stripe API version `2024-06-20`, so
webhook delivery order and the endpoint's event version do not change parsing.
Keep invoice/payment notifications and retry rules configured in Stripe.
The first implementation uses card payments and hosted Checkout. It does not
configure tax collection; configure applicable tax handling before live sales.

Usage consumes daily quota, monthly allowance, signup credit, then prepaid balance. Only paid
full-period invoices grant monthly credits, once per invoice. They expire at the
invoice line's period end and cannot be transferred. No automatic overage charges
or top-ups. Payment failure grants no new month. Cancellation in Account stops
renewal; already paid usage remains until expiry. Resume Pro restores renewal
before the period ends. Payment details opens Stripe for card changes and invoices;
its limited portal configuration is created automatically. Failed reservations refund the
original allowance period. App, product API and authenticated service tools use
the same quota gate; x402 pay-per-call remains separate.

Preserve `transactions.json` and `subscriptions.json` with backups. The former is
the authoritative grant/debit ledger; the latter keeps checkout attempts and
current subscription state. A checkout response lost for more than 23 hours
requires operator reconciliation in Stripe before another attempt: do not erase
its record and risk creating a second subscription. Refunds/disputes and manual
plan changes currently require operator reconciliation; do not assume they revoke
allowances automatically.

Before enabling live sales, run Stripe test-mode purchases, renewal/test-clock,
failed-card recovery, cancellation, duplicate/reordered delivery, process restart
and deletion checks. Confirm model/tool margins and provider-side spending limits,
and verify authenticated Account/Pricing on desktop and mobile.

## Configuration reference

Use `/admin/config` for settings in the browser, or environment variables for
deployment configuration.

### Core

| Variable | Default | What it does |
|---|---|---|
| `ADMIN` / `MU_ADMIN` | first account | Who is admin — comma-separated ids, usernames or emails |
| `MU_OPERATOR_ENABLED` | `false` | Enable operator-only endpoints; human admin account and explicit token permission required. Does not grant OS privileges |
| `TZ` | UTC | IANA timezone for the shared daily image schedule (06:00), such as `Europe/London`. Invalid values fall back to UTC |
| `MU_DOMAIN` | `localhost` | Public domain. Used for the OAuth issuer an MCP client discovers, Stripe returns, ActivityPub actor URLs and mail. Set this if you run behind a proxy |
| `MU_ENV_FILE` | `~/.env`, then `~/.mu/.env` | A dotenv file read at startup; the first that exists wins. Settings saved at `/admin/config` go to `~/.mu/data/settings.json` instead |
| `MCP_REGISTRY_PROOF` | — | Domain-ownership proof served at `/.well-known/mcp-registry-auth` when publishing to the MCP registry — see the MCP registry listing notes in the repository |
| `MU_ENCRYPTION_KEY` | — | Encrypts stored settings at rest |
| `INVITE_ONLY` | off | Require an invite code to sign up |
| `CAPTCHA_SECRET` | — | Signing key for the signup captcha |

### AI provider

One of these is needed for the agent — `mu setup` will prompt for it. Without
one, the agent, chat and AI summaries are off and everything else works.

| Variable | What it does |
|---|---|
| `ANTHROPIC_API_KEY` | Claude |
| `ANTHROPIC_MODEL` | Override the Anthropic model; does not override another selected provider |
| `AI_PROVIDER` | Optional. Which provider to use when this instance has keys for more than one: `anthropic`, `atlascloud`, `gemini`, `openrouter` or `local`. The words `mu setup` writes — `claude`, `atlas`, `ollama` — are accepted too. Set it explicitly when multiple providers have credentials, rather than relying on automatic selection. Setting this puts the agent, chat and the background work on one provider, each using that provider's own model for the job. It filters the model selection menu to that provider. GLM 5.3 and GLM 5.3 Flash follow the selected AtlasCloud/OpenRouter provider, translating `zai-org/` and `z-ai/` IDs automatically. Other explicitly **named** models retain their existing routing — `AGENT_MODEL=deepseek-ai/…` names Atlas whatever this says, because the more specific statement wins. A provider named here with no key stops the request; stored credentials for another provider never authorize fallback |
| `AGENT_MODEL` | Optional. The model the **agent** runs on — the tool-calling loop, which is every question anybody asks it. Separate from `ANTHROPIC_MODEL` because it is a different cost decision: the agent makes several model calls per question while a summary makes one. Naming a model also picks its provider, so `deepseek-ai/deepseek-v4-pro-0813` runs the agent on Atlas Cloud even on an instance with an Anthropic key, and `claude-opus-5` puts the hardest reasoning on the loop. Unset, the agent follows `AI_PROVIDER`; without a preference it checks Anthropic, Atlas Cloud, Gemini, OpenRouter, then the configured OpenAI-compatible endpoint |
| `GUEST_MODEL` | Optional. The model a **signed-out** visitor's question runs on. Unset, a guest gets the quick end of whichever provider this instance uses and a signed-in account gets the thorough one — a better model spends more tokens and more seconds, and somebody who arrived to find one thing out is waiting for it. Set this to put visitors on something specific, including the same model accounts get |
| `MAIL_FORWARD_KEY` | Generated, not set. The key that signs the unsubscribe link in a forwarded message. Written on first use and kept, because that link may be opened a week later and "this link is no longer valid" is the worst thing to say to somebody trying to stop receiving mail. Listed here so it is recognised rather than deleted |
| `ATLASCLOUD_API_KEY` | Atlas Cloud (DeepSeek, Qwen) — also image generation. `ATLAS_API_KEY` still works |
| `GEMINI_API_KEY` | Google Gemini. Not `GOOGLE_API_KEY`, which this instance uses for Maps |
| `GEMINI_MODEL` | Pin a Gemini model. Unset follows `gemini-pro-latest`, which Google keeps pointed at the current generation |
| `ATLAS_MODEL` | Override the Atlas model used when the caller did not name one (default `deepseek-ai/deepseek-v4-pro`) |
| `OPENROUTER_API_KEY` | OpenRouter — one key for Claude, GPT, Gemini and the rest of their catalogue |
| `OPENROUTER_MODEL` | Override the OpenRouter slug (default `openai/gpt-4o-mini`) |
| `IMAGE_MODEL` | Override the image model |
| `OPENAI_BASE_URL` · `OPENAI_API_KEY` | Any OpenAI-compatible endpoint — Ollama, vLLM, llama.cpp |
| `OPENAI_MODEL` | Which model that endpoint serves — `llama3.2`, `qwen2.5`, whatever the machine has pulled. Required with `OPENAI_BASE_URL`: there is no default worth guessing, and the instance says it is not configured rather than asking a server for a model id somebody made up. `mu setup` fills it in by asking the endpoint |
| `MU_LOG_FILE` | Where the log is written (default `~/.mu/logs/mu.log`). Startup printed 313 lines, a hundred of them the framework announcing its own in-memory transport, and the line that mattered — "no model configured" — was third from the top and gone before the scroll stopped. The log goes to a file so the screen can say the address, what is still unconfigured, and where the rest went. Everything still reaches `/admin/logs` either way |
| `MU_LOG_STDOUT` | `true` puts the whole log back on stdout. For Docker and systemd, which capture stdout and expect the log to be there — `docker logs` and `journalctl -u mu` are how an operator reads it, and a file inside a container is not. A choice about where this instance runs rather than about what it should say, which is why it is set rather than guessed |

### Service keys

Each switches on one tool. Without the key that tool is unavailable; the rest
still work.

| Variable | Tool |
|---|---|
| `BRAVE_API_KEY` | `web_search` |
| `YOUTUBE_API_KEY` | `video_list`, `video_search` |
| `GOOGLE_API_KEY` | `places_search`, `places_nearby`, `places_eta` — open-data fallback without it. `places_eta` also needs the **Routes API** enabled on the key, not just Places |

### Texts

An SMS number, from Twilio. Without these the `sms_*` tools refuse and `/sms`
says so; nothing else is affected.

| Variable | Default | What it does |
|---|---|---|
| `TWILIO_ACCOUNT_SID` | — | The **account** SID, which starts with A-C. An API key SID (S-K…) is a credential, not an account: Twilio accepts one for sending, so a key in this slot works and looks configured, and then inbound is refused forever because a webhook signature can only be checked against the account's own auth token |
| `TWILIO_AUTH_TOKEN` | — | The account's auth token. Used to send when there is no API key, and **always** used to verify inbound webhooks. An API key secret will not do |
| `TWILIO_API_KEY` · `TWILIO_API_SECRET` | — | An API key to send with, so the account auth token is not spent on outbound calls. Optional, and it does not replace `TWILIO_AUTH_TOKEN` — signatures still need that |
| `TWILIO_FROM` | — | The numbers texts are sent from and received on, in E.164 (`+447700900123`), comma-separated. **One per country you serve.** The sender is chosen to match the destination — a US long code texting a UK handset is filtered by UK carriers, and a UK number texting a US handset is blocked outright, so a country with no number of its own is refused rather than sent from the wrong one |
| `TWILIO_MESSAGING_SERVICE_SID` | — | A Twilio Messaging Service to send through instead of picking a number here. With **Geomatch** enabled it chooses the sender whose country matches the handset, which is the same rule applied by the party that knows which of your numbers are registered for what. Set `TWILIO_FROM` as well so the page can say what a reply will come from |
| `SMS_COUNTRIES` | `1,44,353,33,49,34,39,31` | Country codes this instance will text, comma-separated. An allowlist rather than a blocklist: a text to a premium range can cost fifty times what one to a mobile does, and those ranges are where revenue-share fraud lives |
| `SMS_DAILY_LIMIT` | `5` | Messages one account may send in a day, on top of the per-message price. It is `limit_env` on `sms_send` in `quota.json`, where the number lives. **Set it to `0` to stop sending entirely** — that is the kill switch, and it is the same setting rather than a second one because an operator reaching for it is in a hurry |
| `SMS_NEW_ACCOUNT_LIMIT` | `3` | The same cap for an account less than a day old. Signing up is free and takes a minute, so this is the only thing between a script and the full allowance |
| `SMS_KNOWN_ONLY` | off | Restrict sending to numbers the caller already knows — someone in their contacts, a number they verified as their own, or one that texted them first. Off, because `contacts_add` takes any number and defeats it in one call, and because it stopped an agent doing the ordinary thing. On, it is a real brake for an instance that wants one |
| `SMS_VERIFY_INBOUND` | on | Inbound SMS/WhatsApp messages always require a valid Twilio signature; this legacy switch cannot disable that check. Configure the account's `TWILIO_AUTH_TOKEN` even when outbound sends use an API key. |
| `SMS_DEFAULT_COUNTRY` | — | Country code assumed for a number written without one. Unset, a number with no `+` is refused rather than guessed |
| `WHATSAPP_ENABLED` | enabled unless explicitly `false` | Set to `false` to disable WhatsApp inbound processing and sending, and hide its number and links from client discovery, Connect and contact cards. Keeps `TWILIO_WHATSAPP_FROM` intact. Leave unset or set `true` to restore the configured integration. Also editable in Admin Config. |
| `TWILIO_WHATSAPP_FROM` | — | The WhatsApp sender, in E.164 (`+447700900123`). WhatsApp rides the same Twilio account and the same webhook as SMS — point Twilio's WhatsApp sender at `https://<your domain>/whatsapp/twilio` — and configuring this enables it unless `WHATSAPP_ENABLED=false`. One number, not a list: a WhatsApp sender is registered with Meta against a business rather than routed by country, so the matching `TWILIO_FROM` needs does not apply. Unset, WhatsApp is off and nothing offers it |
| `WHATSAPP_DAILY_LIMIT` | `20` | WhatsApp messages one account may send in a day. Higher than `SMS_DAILY_LIMIT` and priced lower because Meta bills a 24-hour conversation rather than each message. It is `limit_env` on `whatsapp_send` in `quota.json`. **Set it to `0` to stop sending on WhatsApp** without touching texts |

WhatsApp has one rule SMS does not, and it is Meta's rather than ours: this
instance may only message somebody in the 24 hours after they last wrote to it.
Outside that window only templates approved in advance are accepted, and there
are none here — so a message sent late is refused with the reason rather than
handed to the provider to drop. In practice this is invisible, because replying
to somebody who just wrote is what the window is for.

Senders have to be registered before they will deliver. In the **US**, an
unregistered long code is blocked by every major carrier: either a toll-free
number with toll-free verification (free, reviewed in days, two-way, the
shortest path for low volume) or a 10DLC long code with a brand and campaign
registered through The Campaign Registry. In the **UK**, use a virtual mobile
number (`+447…`) rather than an alphanumeric sender ID — an alphanumeric sender
cannot receive, which means no replies and no way for anyone to text STOP, and
US carriers reject alphanumeric senders outright.

| `TWILIO_WEBHOOK_URL` | — | The inbound webhook address exactly as configured on the number. Only needed if the signature check is failing: it covers the URL Twilio called, which behind a proxy is not the URL this process sees, and a mismatch drops every inbound message while Twilio reports it as 11200 |

Point each number's inbound webhook at `https://<your domain>/sms/webhook`. The
request is verified against `TWILIO_AUTH_TOKEN`, so nothing else needs opening
up, and `MU_DOMAIN` has to match what Twilio calls or the signature will not
check out.

Delivery receipts need no setting at all. Every outgoing message asks Twilio to
post back to `https://<your domain>/sms/status` as it moves — queued, sent,
delivered, failed — and the address is derived from `TWILIO_WEBHOOK_URL` where
that is set and `MU_DOMAIN` otherwise, because a third place to write down where
this instance lives is a third place for the three to disagree. Nothing is
asked for on a box with no reachable address, so a development instance does not
send Twilio retrying at localhost. The receipts are what make "it was slow"
answerable: sending is not delivering, `twilio.Send` returns when the provider
*accepts* a message, and without a receipt the record stops there — a text that
took a second and one that took a minute look identical. `/sms` shows the gap
when there is one, and says nothing when there is not.

### File storage

Uploaded files and archived images go to the local disk by default, under
`~/.mu/data`. On a hosted instance that is usually the wrong place: the volume
is small, is not replicated, and goes when the machine does. Set these and they
go to any S3-compatible bucket instead — DigitalOcean Spaces, Cloudflare R2,
Backblaze B2, MinIO, S3.

| Variable | Default | What it does |
|---|---|---|
| `S3_ENDPOINT` | — | For anything that is not AWS, e.g. `https://lon1.digitaloceanspaces.com`. Leave empty for AWS |
| `S3_BUCKET` | — | Bucket name |
| `S3_ACCESS_KEY_ID` · `S3_SECRET_ACCESS_KEY` | — | Credentials used by Files and backups |
| `S3_REGION` | `us-east-1` | Region for the signature. DigitalOcean uses the datacentre slug, e.g. `lon1` |

`S3_BUCKET` and both credentials must be set. For AWS, leave `S3_ENDPOINT`
empty; for another S3-compatible provider, set its endpoint as well. Anything
less is a misconfiguration: it is logged and the instance keeps using the disk
rather than failing.

Switching an instance that already holds files is safe. New writes go to the
bucket, and a read that misses there falls back to the disk, so files stored
before the change keep working with no migration. Copy them across at your
leisure; the fallback stops mattering once you have.

Keep the bucket **private**. Files are served through Mu, which checks who is
asking — a public bucket would let anyone holding an object URL route around
that. The bucket is shared by durable services with fixed namespaces: Files
uses `files/` and off-box backups use `backups/`.

### Mail

| Variable | Default | What it does |
|---|---|---|
| `MAIL_DOMAIN` | — | The domain you send and receive as |
| `MAIL_DAILY_LIMIT` | `50` | Messages one account may send in a day — to somebody here or outside, since there is one price for both. It is `limit_env` on `mail_send` in `quota.json`. Writing to yourself or to your own agent is not a send and does not count against it. A self-hosted instance that wants no ceiling raises it here; one that wants no charge sets `CREDIT_COST_MAIL=0`, and one with no Stripe and no x402 is never charged anyway |
| `MAIL_PORT` | `2525` | SMTP listener — `25` in production, `off` to have none |
| `IMAP_PORT` | `1143` | IMAP listener — `143` in production, `off` to have none. See [Reading your mail in a mail client](CHANNELS.md#reading-your-mail-in-a-mail-client) |
| `SUBMISSION_PORT` | `1587` | SMTP submission, so a mail client can send — `587` in production, `off` to have none |
| `IMAP_PUBLIC` | — | What `/inbox/imap` tells people to connect to, `host:port`. The listener runs in the clear behind a terminator, so the bound port is usually not the port a client dials; unset, the page offers `993` and names the local port beside it |
| `SUBMISSION_PUBLIC` | — | The same for outgoing. Unset, the page offers `465` |
| `MAIL_SELECTOR` | `default` | DKIM selector, the `<selector>._domainkey` DNS record |
| `DKIM_PRIVATE_KEY` | — | DKIM signing key |
| `SMTP_RELAY_HOST` | — | Hand outbound mail to a submission server instead of delivering it to the recipient's MX. `host` or `host:port`, 587 assumed. See [Outbound deliverability](CHANNELS.md#outbound-deliverability) |
| `SMTP_RELAY_USER` | — | Username for the relay. No username means no AUTH |
| `SMTP_RELAY_PASS` | — | Password for the relay |
| `MAIL_WHITELIST` | — | Domains you accept mail from, comma separated: `acme.com, partner.co.uk`. Merged with a built-in list of company and infrastructure domains; consumer domains are deliberately absent. Live — no restart |

### Notifications

Mail, briefings and answers can turn up on a phone with the page closed. Nothing
to configure: the first time somebody turns it on, this instance mints its own
signing key and keeps it.

| Variable | Default | What it does |
|---|---|---|
| `VAPID_PRIVATE_KEY` | minted on first use | The key that signs push requests, base64url. Set it only to move an instance without invalidating what people have already subscribed — a browser binds its subscription to the public half, so a new key silently stops every existing device receiving anything |

The payload is encrypted end to end (RFC 8291): the push service — Google's,
Apple's, Mozilla's — forwards bytes it cannot read. It does learn that a
notification went to a device, and when.

Turning it on is a button on `/account`, per device, and the browser asks before
anything is stored. It needs HTTPS: a service worker will not register over
plain HTTP, except on `localhost`.

### The daily briefing

| Variable | Default | What it does |
|---|---|---|

DNS records are in the [mail guide](CHANNELS.md#mail), and [Who is allowed to send you mail](CHANNELS.md#who-is-allowed-to-send-you-mail) is the whole inbound rule.

### Social

| Variable | Default | What it does |
|---|---|---|
| `SOCIAL_ATPROTO` | off | `true` to watch the open social network — Bluesky's public firehose — for posts worth surfacing on `/social` |

Off unless you turn it on. Everything else in Mu works with no configuration;
this one does not, because pulling strangers' posts into your instance is a
decision about what you are willing to publish, and it is yours to make.

No key and no account: the firehose is public JSON over a websocket. What
arrives is about three million posts a day, so almost all of the work is
refusing them — English, not a reply, long enough to stand alone, pointing at
something, in one of the categories the news is already sorted by, and not an
advert or a repost bot. What survives is scored, cut to one per category and
one per author, and then read by your model, which picks at most three. It is
allowed to pick none.

**It does not hold the connection open.** Ninety seconds every fifteen minutes
is enough to find far more than three worth publishing, and holding it open the
rest of the time costs 2.6 GB a day to fill a buffer that gets thrown away.
Four fifths of what does arrive is refused on the raw bytes, before it reaches
a JSON parser. Budget roughly 150 MB a day and one model call every fifteen
minutes.

Without a model configured the shortlist is published in score order, which
works but is noticeably worse — the arithmetic cannot tell a news story from a
press release, and both look identical to it.

### Channels

| Variable | What it does |
|---|---|

### Sign-in

| Variable | What it does |
|---|---|
| `GOOGLE_CLIENT_ID` · `GOOGLE_CLIENT_SECRET` | Google sign-in |
| `GOOGLE_REDIRECT_URI` | Defaults to `<your-origin>/oauth2/callback` |
| `PASSKEY_ORIGIN` · `PASSKEY_RP_ID` · `PASSKEY_EXTRA_ORIGINS` | WebAuthn — derived from the request when unset |

### Payments

Callers pay in credits. `STRIPE_*` is the one that matters: set those keys and
people can buy credits by card. The `X402_*` and chain variables configure
stablecoin settlement, which funds credits — the way in is still MCP with a
token.

| Variable | What it does |
|---|---|
| `X402_PAY_TO` | Your wallet address — receives x402 payments |
| `X402_HOST` | Optional second public hostname for the x402 machine interface; requests on this host keep that origin in MCP, OAuth and payment URLs |
| `X402_NETWORK` · `X402_VERSION` | The advertised pair. Default `eip155:8453` + `2`. CDP settles `base`+`1` too, and that pair works — but the discovery index carries only v2 entries, so a v1 server is payable and unfindable. Both are live: change them at `/admin/config` and the next request uses them |
| `X402_ASSETS` | Accepted tokens (default USDC) |
| `TFL_APP_KEY` | Optional. Transit works with no key at all — this only raises TfL's rate limit, and one is free to register at api-portal.tfl.gov.uk |
| `TRANSIT_FEEDS` | Optional. Published timetables to load, comma separated, named by agency or place: `reading buses, bart, vbb`. Matched against the Mobility Database catalogue, which lists about 1,160 keyless feeds. Nothing is downloaded that is not named here — a feed is tens of megabytes. Each is checked once a day and only re-fetched when it has actually changed, and a feed that fails to download or build leaves the previous one serving |
| `BODS_API_KEY` | Optional. Bus Open Data Service key, free at data.bus-data.dft.gov.uk. Live bus positions across England, which is what `transit_buses` answers from. Without it transit still has stops and timetables; it just cannot say where anything is |
| `LDBWS_TOKEN` | Optional. National Rail Live Departure Boards token, free at realtime.nationalrail.co.uk. Powers `transit_trains` — the board at any British station. Not the Darwin real-time feed, which is a Kafka consumer group and a different kind of program: this is request in, board out, and a restart loses nothing |
| `BROWSER_URL` | Optional. A Chrome DevTools endpoint for `/browser` — a Chromium container on this host, a box on the network, or a hosted browser. This is what keeps Mu a single binary: the dependency is an address rather than a program on this disk. Neither this nor `CHROME_PATH` is needed on a machine that already has Chromium or Chrome installed; the service looks on the PATH first |
| `CHROME_PATH` | Optional. Path to a particular Chromium, when the one found on the PATH is not the one you want. Unset, the service looks for `chromium`, `chromium-browser`, `google-chrome` and the macOS bundle, so an installed browser needs no configuration. `BROWSER_URL` wins over both |
| `ALERTS` | Optional, default on. `off` stops this instance telling you anything. What it watches and what it would say is at `/admin/alerts`, including a button that sends one so you can check delivery works |
| `ALERT_CALLS_PER_HOUR` | Optional, default 5000. Tool calls an hour across the whole instance before you are told. `0` stops watching it |
| `ALERT_ACCOUNT_CALLS_PER_HOUR` | Optional, default 1000. The same for any one account, which is the one that catches an agent in a retry loop — expensive long before it is a noticeable share of a small instance's traffic. `0` stops watching it |
| `ALERT_DISK_PERCENT` | Optional, default 85. How full the disk holding the data directory may get. This is the one that is an outage rather than information: mail stops being accepted and the record stops being written. `0` stops watching it |
| `ALERT_COOLDOWN_MINUTES` | Optional, default 360. How long after an alert fires before the same one may fire again. This is what makes a threshold safe to set low — crossing one costs a message, not a message every five minutes until it is fixed |
| `SHELL_IMAGE` | Optional, default `alpine:3.20`. The image `/shell` gives each account a machine of. Small on purpose — an operator who has not thought about it does not silently get a gigabyte pulled the first time an agent tries something. Set it to what the work needs: `golang:1.26` for a Go checkout, `python:3.13-slim`, or an image of your own |
| `SHELL_MEMORY` · `SHELL_CPUS` · `SHELL_PIDS` | Optional. What one machine may have. Memory defaults to a **quarter of what the host has**, floored at `256m` and capped at `2g` — not a flat number, because a flat `2g` on a 2GB VM is the whole box: the container stays inside its own cgroup while taking every free page, and the host's OOM killer then picks the largest process, which is the Mu server. CPU defaults to `1`, or `0.5` on a single-core box, for the same reason. Processes default to `512`, against a fork bomb. Docker's own syntax, so `512m` and `0.5` are fine, and setting any of them wins over the derived value. Swap is disabled — without that a container gets swap equal to its memory for free, and the symptom is the box thrashing while the container stays inside its limit |
| `SHELL_SHARED` | Disabled for account isolation. If previously enabled, new shell/SSH execution is refused. Migrate existing shared workspaces to per-account containers before unsetting this option. Shared volumes are not deleted or moved automatically. |
| `SHELL_MAX_MACHINES` | Optional, default half the host's memory divided by what one machine takes, minimum 1. How many machines may run at once. Each holds its memory cap whether or not anybody is using it, so a box that fits two cannot host five however cheap a command is. Starting one past the cap stops the idlest machine rather than refusing the caller — the volume is untouched and their next command starts it again |
| `SHELL_NETWORK` | Optional, default `bridge`. `none` gives machines no network at all. The default is on because a machine that cannot fetch a dependency or push a branch cannot do the thing this is for — what the container bounds is the host, not the internet |
| `SHELL_MAX_SECONDS` | Optional, default 600. The longest one command may run, whatever it asked for. A command with no timeout of its own gets 120 |
| `XMPP_PORT` | **On by default at `:5222`**; set it to `off` to close the door. A port to answer XMPP on, so `asim@your.domain` is a chat address as well as a mailbox — one account, one local part, reachable two ways. Conversations, Dino, Gajim and Monal are clients for it. Sign in with your username and an access token as the password, the same credential IMAP and submission take. XMPP supports agent JIDs such as `agent@your.domain` and `you+research@your.domain`. Email has a separate policy: only `agent@your.domain` invokes the assistant; personal plus-addresses are mail filters. Nothing in Mu terminates TLS, so bind it to loopback and put the proxy on 5223 — see the nginx `stream {}` section above, which is the same arrangement IMAP and submission use, plus the `_xmpps-client._tcp` SRV record a client needs to find it. Federation is on the separate `XMPP_S2S_PORT` below |
| `XMPP_S2S_PORT` | **On by default at `:5269`**; set it to `off` to keep this instance to itself. The federated port, where other XMPP servers connect so that `asim@your.domain` can message somebody on any Prosody, ejabberd or Openfire deployment, and they can message back. Servers prove which domain they are by dialback (XEP-0220): they hand over a key and this instance opens its own connection to the domain they claim and asks whether the key is theirs, so a server that cannot receive mail at the domain it claims cannot pass. That means two things for DNS. Publish `_xmpp-server._tcp.your.domain` pointing at this host on 5269, or make sure `your.domain` itself resolves to it, because that is where other servers look and where the verification call comes back to. And unlike `XMPP_PORT` this one faces the internet directly rather than through the proxy: STARTTLS is offered with a self-signed certificate generated on first use, which is correct here because dialback and not the certificate is what proves the domain — every federated server skips certificate verification on this port for that reason. Turn it off if you want your own people on XMPP without accepting connections from every other server on the internet |
| `SHELL_SSH_PORT` | Optional, **off by default**. A shared SSH port for Shell (`ssh -p 2222 you@host`) and Files (`sftp -P 2222 you@host`). Mu is the SSH server; no `sshd` runs inside a container and SFTP never exposes the container or host filesystem. Keys are account credentials shared by both doors, with no passwords, and the username is ignored: which key signed the handshake says who you are. There is no default because `22` on the host is the host's own `sshd` and taking it by accident locks you out. Register the same key at `/files` or `/shell` |
| `SHELL_IDLE_MINUTES` | Optional, default 30. How long a machine may sit doing nothing before it is stopped. Stopping is not deleting: the `/work` volume is untouched and the next command starts it again in about a second. This is what bounds the memory of machines nobody is using, which a price on commands would not have — the cost is the idle container rather than the calls |
| `OS_MAPS_KEY` | Optional. Ordnance Survey Data Hub key, for `/maps` — the basemap under anything spatial. Free tier at osdatahub.os.uk. Britain only. Without it the service still serves every tile this instance has already fetched, so a lapsed key degrades to the region you have already used rather than to nothing. Tiles are free to callers; what bounds them is `TILE_FETCH_PER_HOUR` |
| `TILE_FETCH_PER_HOUR` | Optional, default 2000. How many tiles one account may make this instance fetch from Ordnance Survey in an hour. Tiles already held are served without limit and without a session, because serving one again costs nothing — this bounds only what is spent upstream. Raise it to seed a region on purpose |
| `X402_SERVERS` | Other MCP servers this instance may pay, as `name=url` — read by the outbound client, which no tool currently exposes |
| `CDP_API_KEY_ID` · `CDP_API_KEY_SECRET` | Coinbase facilitator credentials |
| `STRIPE_SECRET_KEY` · `STRIPE_WEBHOOK_SECRET` | Card top-ups for credits. Point the endpoint at `https://<your domain>/stripe/webhook` and subscribe it to `checkout.session.completed` (plus the events under Monthly subscriptions when enabled). It is belt and braces rather than the only route: the return from Stripe settles a purchase too, so a webhook that is missing, misconfigured or signed with the wrong secret no longer means the card is charged and nothing happens |
| `BASE_RPC_URL` | The node balances are read from. Optional: unset, it uses the public Base endpoint, which is rate-limited but on the right chain. Point it at a Base node and nothing else — an Alchemy key is per-chain, so an Ethereum endpoint here finds no USDC contract at the address, returns nothing, and reports every wallet on the instance as empty with no error at all |

The webhook used to be at `/wallet/stripe/webhook`, and that path still answers
so an instance upgrading does not lose a top-up between the deploy and the
dashboard edit. Move it when convenient; the old one goes away once nothing is
arriving there. It is named for Stripe rather than for whichever page shows a
balance because a webhook URL is a contract with somebody outside this process:
it is configured once, possibly by somebody who has since left, and it should
not need changing because we rearranged our own routes.

### Prices and limits

Prices are data, not code. They live in `quota.json` at the top of the repo,
embedded into the binary by `main.go`: one entry per operation, with its cost in
credits, the label the cost tables show, and the environment variable that
overrides it.

Three ways to change one, in increasing order of precedence:

1. Edit `quota.json` and rebuild.
2. Drop a `quota.json` in the data directory (`~/.mu/data/quota.json`). It is
   merged entry by entry, so a file naming one operation changes that one and
   leaves the rest alone — no restart needed if you call the reload.
3. Set the variable named on the entry — `CREDIT_COST_SEARCH=2`,
   `CREDIT_COST_IMAGE=20`. This is the container-friendly one.

An override of `0` is ignored, because an unset variable and one set to `"0"`
look the same to a container and a price silently dropping to free is the wrong
way to fail. Make something free in the file.

The full operation price list is in `quota.json`; account billing shows usage.

| Variable | Default | What it does |
|---|---|---|
| `POST_LIMIT_PER_HOUR` · `NEW_POST_LIMIT_PER_HOUR` | — | Posting rate limit, and the tighter one for new accounts |
| `VIDEO_SEARCH_PER_HOUR` | 20 | YouTube searches one account may run per hour |
| `VIDEO_SEARCH_PER_DAY` | 80 | YouTube searches this instance may run per day, kept under the API's own quota |
| `SIGNUP_MAX_PER_IP` · `SIGNUP_WINDOW_HOURS` | — | Signups allowed per IP, and the window |
| `GUEST_MAX_PER_CLIENT` · `GUEST_WINDOW_MINUTES` | 40 · 60 | Free calls one browser may make. This is the fair share: it is per browser, so it can be sized for a person rather than for a building |
| `GUEST_MAX_PER_IP` | 300 | Free calls one address may make. The backstop behind the per-browser share, for a caller who clears the marker cookie to get a new one. Wide on purpose — an address may be a cafe, a campus or a phone network |
| `TRUSTED_PROXY` | — | Comma-separated addresses or CIDRs whose `X-Forwarded-For` is believed. Unset, loopback and private peers are trusted, which covers nginx or Caddy on the same host or network. Set this when your proxy has a public address (a cloud load balancer, Cloudflare) — otherwise every visitor is counted as the proxy. Never leave it naming a hop anyone can reach: a caller whose forwarding header is believed can pick a new address per request and reset every limit keyed on one |
| `X402_FACILITATOR_URL` | Coinbase | x402 facilitator to settle through |

### Runtime

| Variable | Default | What it does |
|---|---|---|
| `MU_REGISTRY` | in-process | `mdns` puts services on the local network — note it *announces* every service this process hosts |
| `MU_ADVERTISE` | loopback | Address to advertise when the registry is networked |
| `MU_USE_SQLITE` | `1` | SQLite with FTS5 for the search index. On by default — set it to `0` for the older file store, a map read end to end on every query. Switching decides where the *index* lives and nothing else; the first boot after turning it on migrates `index.json` into it once, so an instance that has been running keeps everything it had indexed |
| `MCP_GATEWAY_ADDR` | — | Run go-micro's MCP gateway on its own port |
| `PUBLIC_URL` · `APP_URL` | — | Public origin, when it can't be derived |
| `TOR_ONION` | — | Onion address, shown in the footer |
| `OPINIONS` | on | The blog writes an opinion piece a day, and the daily briefing is written from them. Each piece is a research pass and a generation billed to the instance's own account, so `off` disables them — the briefing then falls back to summarising the raw feeds |
| `CREDIT_COST_AGENT_RUN` | 3 | What one answered question costs the caller, in credits. Set to 0 to make the agent free, which is what a self-hosted instance paying its own model bill usually wants — an instance with no payments configured charges nothing regardless |
| `OPINIONS_PER_DAY` | 1 | How many pieces a day. The topic picked is whichever has gone longest without one, so the whole topic list is covered over as many days as there are topics. Raise it for a busier blog; never exceeds the number of topics, and is capped at 8 |

### CLI

The same binary is the client. `mu --serve` runs an instance; `mu news list`,
`mu ask`, `mu agent` call one — and **which one is a separate question from
whether this machine is running a server.**

By default the CLI calls **https://micro.mu**, the instance this project runs.
That is deliberate: the first thing anybody does with a command-line tool is
run it, and "no server configured" is a worse first answer than a result. It
does mean that if you have just installed your own instance and typed
`mu news list`, you have called somebody else's — so point it at yours:

```bash
mu login https://your.host      # saves the address and a token, for good
```

Everything after that goes to your instance: the tool commands, `mu ask`,
`mu agent` renting tools over x402, and `mu x402 call`.

To check what is in force, and what decided it:

```bash
$ mu config get
url=https://your.host (/home/you/.config/mu/config.json)
token=***
```

The four sources, strongest first:

| Source | Example | Scope |
|---|---|---|
| `--url` | `mu --url https://other.host news list` | One command. Must come *before* the tool name — `mu web fetch --url …` is the fetch tool's own argument |
| `MU_URL` | `MU_URL=https://other.host mu news list` | One shell |
| Config file | written by `mu login <url>` | Permanent, per user |
| Default | `https://micro.mu` | When nothing else says |

| Variable | What it does |
|---|---|
| `MU_TOKEN` | Personal Access Token. Overrides the saved one |
| `MU_URL` | Instance to talk to. Overrides the saved one |
| `MU_NO_COLOR` | Disable colour output |

A token belongs to the instance that issued it, so changing the address
without changing the token gets you a 401 — `mu login <url>` does both.

### Object storage and generation policy

| Variable | Default | What it does |
|----------|---------|--------------|
| `S3_BUCKET` | — | Shared bucket for durable storage. Files use `files/`; off-box backups use `backups/` |
| `S3_REGION` | `us-east-1` | Region of the bucket |
| `S3_ENDPOINT` | — | For anything that is not AWS — R2, Backblaze, MinIO. Leave empty for AWS |
| `S3_ACCESS_KEY_ID` | — | Access key for the bucket. Files need read, write and delete access; backups use the same credentials |
| `S3_SECRET_ACCESS_KEY` | — | Secret key |
| `BACKUP_S3` | `false` | Whether backups are pushed to the bucket above under `backups/` |

### Operator endpoint policy

`MU_OPERATOR_ENABLED=true` explicitly enables endpoints declared operator-only.
It defaults to disabled. Access also requires an authenticated human account with
`Admin` set and `Agent` unset. Personal access tokens must carry an explicit
`operator` permission as well as any required service scope; existing unscoped
tokens do not inherit it. Calls within Mu agent runs and x402 identities are
refused. This policy does not install host-management capabilities or grant OS
privileges. Mu's existing service user and sandbox permissions are unchanged.

The separately configured tools host includes `X-Mu-Catalogue-Version` and
`X-Mu-Tool-Count` headers. Reconnect MCP clients to refresh discovery after a
deployment; the primary host now lists Agent, Work and Inbox operations.

### Flight schedules and estimates

Set `AVIATIONSTACK_API_KEY` in `/admin/config` to enable scheduled, estimated and actual flight times on `/flights`. Without it, the existing aircraft tracking still works. Lookups return up to 20 provider records; missing estimates are shown as unavailable. Set the `flights_status` price in `quota.json` to cover your provider plan (the shipped price is 3 credits per lookup).

### Optional Google connections

Set `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET`, and register the instance's
`https://<host>/oauth2/callback` redirect URI in Google Cloud. Sign-in requests
only identity. Calendar, Contacts, Gmail and Drive each have a separate Connect
button under Account and request read-only access only after the user chooses it.
Enable Calendar API, People API, Gmail API and Drive API for the connections you
intend to offer. Gmail and Drive read-only scopes are restricted scopes; external
production OAuth applications require Google's applicable verification. Testing
apps are limited to the test users and token lifetime set by Google.

Previously revoked grants cannot be restored; users must reconnect. The agent
can search Gmail and Drive and read selected messages or text files (including
text exports of Docs, Sheets and Slides). There is no background bulk import.
Looked-up content used to answer a prompt is sent to the configured AI provider,
and answers may be retained in the conversation. Disconnect removes the local
grant and requests revocation from Google.

Automatic AI content generation (daily images, public digests, news summaries, tagging and arrival triage) is disabled. Personal daily brief schedules and explicit requests remain available. Model failures do not retry on another provider.

### Checking Codex availability

#### Isolated admin preview

Codex is an opt-in provider for an administrator's direct conversations. It
does not replace the site provider, change scheduled work or require a separate
agent. The Account page has a **Codex preview** section for admins only.

The preview currently requires Linux, Bubblewrap (`apt install bubblewrap` on
Debian/Ubuntu), permitted unprivileged user namespaces, and the native
Codex **0.156.1** executable (standalone or installed through npm). `CODEX_BINARY` can specify its absolute path; the
default is `codex` on PATH. For an npm installation, Mu resolves the launcher to its bundled Linux binary
without running JavaScript or exposing the npm package tree to the sandbox.
Install optional dependencies when using npm (`npm install -g @openai/codex@0.156.1`).
Set the same executable path in the server environment and the setup shell.

As the OS user that runs Micro:

```sh
mu codex login
mu codex check
```

Login uses a separate `~/.mu/codex-auth` credential store. It does not copy or
modify `~/.codex`, its sessions, memories, skills or personal configuration.
Check verifies the actual sandbox, the pinned protocol and a real GPT-6-Astra
request with a harmless test tool. It consumes a small amount of the signed-in
ChatGPT allowance. Only a successful check unlocks the Account setting. A failed
check leaves the preview unavailable; there is no unsandboxed fallback. Do not
disable system-wide security controls to work around a failed namespace check.

After it passes, open **Account → Codex preview → Enable for my conversations**.
Continue using the normal Micro conversation UI. Choose **Use the site provider**
in the same section to revert. No other account is switched. Revoking admin
access also disables the preview at dispatch time. The general `AI_PROVIDER`
setting remains unchanged; Codex is not offered as a site-wide provider yet.

Each request launches a fresh Bubblewrap filesystem/process sandbox and an
ephemeral Codex thread. It mounts system runtime libraries, CA/DNS files, the
Codex executable and a temporary state directory—not Micro's data, personal home
directories, host sockets or personal Codex state. Only the separate login
credential is copied into temporary state; refreshes are serialized and written
back, and the temporary session is removed when the request ends. Codex memories,
personal apps/connectors, plugins, native shell/browser/computer tools and
environment access are disabled. Micro supplies history and handles only the
advertised account-scoped service tools through its existing authorization chain.

Networking remains available for OpenAI; Bubblewrap here is a filesystem and
process boundary, not an outbound-network firewall. The authentication remains
tied to the chosen ChatGPT account and its allowance. This preview is not a claim
that a ChatGPT login is interchangeable with an API credential. A separate
OpenAI account provides additional separation of account-level authority.

No Codex process, model call or readiness check runs during server startup or
page rendering. The pilot serializes requests sharing its credential to avoid
refresh-token races. Upgrading Codex requires revalidating the integration and
running the check again; an unexpected binary version is refused on each request.

#### Existing personal CLI login

On the machine and OS account that will run Micro, install the Codex CLI and
sign in with `codex login`. Then run `mu codex status`. This starts a temporary
local App Server over stdio, checks the authentication type and lists models
available to that account. It does not run a prompt or change Micro's provider.
Use the returned model identifiers; a model name shown in ChatGPT does not by
itself establish availability through Codex.

This generic status probe does not validate the isolated preview. Use the setup
and check above before enabling it. Do not expose an unauthenticated App Server
socket or give a hosted user's agent access to the server shell.
See the [App Server protocol](https://learn.chatgpt.com/docs/app-server) and
[authentication guide](https://learn.chatgpt.com/docs/auth).
