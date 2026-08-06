# Configuration

Every setting is an environment variable, or set from `/admin/env` in the browser
once you are admin. Settings read the environment first and fall back to the
stored value, so an env var always wins.

Nothing here is required to start. Mu runs with no configuration at all; each key
switches on the feature next to it.

Every variable below is one the code actually reads — `TestEveryConfigVarIsDocumented`
checks this page against every `settings.Get` and `os.Getenv` in the source, in
both directions, so it can neither fall behind nor accumulate settings that no
longer exist.

## Core

| Variable | Default | What it does |
|---|---|---|
| `ADMIN` / `MU_ADMIN` | first account | Who is admin — comma-separated ids, usernames or emails |
| `MU_DOMAIN` | `localhost` | Public domain. Used for the OAuth issuer an MCP client discovers, Stripe returns, ActivityPub actor URLs and mail. Set this if you run behind a proxy |
| `MU_ENV_FILE` | `~/.env`, then `~/.mu/.env` | A dotenv file read at startup; the first that exists wins. Settings saved at `/admin/env` go to `~/.mu/data/settings.json` instead |
| `MCP_REGISTRY_PROOF` | — | Domain-ownership proof served at `/.well-known/mcp-registry-auth` when publishing to the MCP registry — see [Listing](LISTING.md) |
| `MU_ENCRYPTION_KEY` | — | Encrypts stored settings at rest |
| `INVITE_ONLY` | off | Require an invite code to sign up |
| `CAPTCHA_SECRET` | — | Signing key for the signup captcha |

## AI provider

One of these is needed for the agent — `mu setup` will prompt for it. Without
one, the agent, chat and AI summaries are off and everything else works.

| Variable | What it does |
|---|---|
| `ANTHROPIC_API_KEY` | Claude |
| `ANTHROPIC_MODEL` · `ANTHROPIC_PREMIUM_MODEL` | Override the default models |
| `ATLAS_API_KEY` | Atlas Cloud (DeepSeek, Qwen) — also image generation |
| `IMAGE_MODEL` | Override the image model |
| `OPENAI_BASE_URL` · `OPENAI_API_KEY` | Any OpenAI-compatible endpoint — Ollama, vLLM, llama.cpp |
| `AGENT_NATIVE` | `off` falls back to the hand-rolled planner |
| `AGENT_NATIVE_STREAM` | `off` forces the streaming UI onto the planner |

## Service keys

Each switches on one tool. Without the key that tool is unavailable; the rest
still work.

| Variable | Tool |
|---|---|
| `BRAVE_API_KEY` | `web_search` |
| `YOUTUBE_API_KEY` | `video_list`, `video_search` |
| `GOOGLE_API_KEY` | `places_search`, `places_nearby`, `places_eta` — open-data fallback without it. `places_eta` also needs the **Routes API** enabled on the key, not just Places |

## File storage

Uploaded files and archived images go to the local disk by default, under
`~/.mu/data`. On a hosted instance that is usually the wrong place: the volume
is small, is not replicated, and goes when the machine does. Set these and they
go to any S3-compatible bucket instead — DigitalOcean Spaces, Cloudflare R2,
Backblaze B2, MinIO, S3.

| Variable | Default | What it does |
|---|---|---|
| `S3_ENDPOINT` | — | Bucket endpoint, e.g. `https://lon1.digitaloceanspaces.com`. Unset means the local disk |
| `S3_BUCKET` | — | Bucket name |
| `S3_ACCESS_KEY` · `S3_SECRET_KEY` | — | Credentials |
| `S3_REGION` | `us-east-1` | Region for the signature. DigitalOcean uses the datacentre slug, e.g. `lon1` |

`S3_ENDPOINT` and `S3_BUCKET` must both be set, with both credentials. Anything
less is a misconfiguration: it is logged and the instance keeps using the disk
rather than failing.

Switching an instance that already holds files is safe. New writes go to the
bucket, and a read that misses there falls back to the disk, so files stored
before the change keep working with no migration. Copy them across at your
leisure; the fallback stops mattering once you have.

Keep the bucket **private**. Files are served through Mu, which checks who is
asking — a public bucket would let anyone holding an object URL route around
that.

## Mail

| Variable | Default | What it does |
|---|---|---|
| `MAIL_DOMAIN` | — | The domain you send and receive as |
| `MAIL_PORT` | `2525` | SMTP listener — `25` in production |
| `MAIL_SELECTOR` | `default` | DKIM selector, the `<selector>._domainkey` DNS record |
| `DKIM_PRIVATE_KEY` | — | DKIM signing key |

DNS records are in [Installation](INSTALLATION.md).

## Channels

| Variable | What it does |
|---|---|
| `DISCORD_BOT_TOKEN` | Discord bot |
| `TELEGRAM_BOT_TOKEN` | Telegram bot |
| `WHATSAPP_TOKEN` · `WHATSAPP_PHONE_ID` · `WHATSAPP_APP_SECRET` · `WHATSAPP_VERIFY_TOKEN` | WhatsApp Business API |

## Sign-in

| Variable | What it does |
|---|---|
| `GOOGLE_CLIENT_ID` · `GOOGLE_CLIENT_SECRET` | Google sign-in, and the calendar connection below |
| `GOOGLE_REDIRECT_URI` | Defaults to `<your-origin>/oauth2/callback` |

The same credentials let a signed-in person attach their Google Calendar, so
`events_free` counts the week they actually have rather than only what Mu
scheduled. Nobody is asked at signup — the ask appears on `/events`, and in the
agent's reply when it had to answer from one calendar.

Two things must be true in the Google Cloud project for it to work: the
**Google Calendar API** enabled, and `.../auth/calendar.readonly` listed on the
OAuth consent screen. That scope is *sensitive*, so a public app needs Google's
verification before anyone outside your test users can grant it. Read-only is
deliberate — Mu never writes to a calendar it does not own.
| `PASSKEY_ORIGIN` · `PASSKEY_RP_ID` · `PASSKEY_EXTRA_ORIGINS` | WebAuthn — derived from the request when unset |

## Payments

Callers pay in credits. `STRIPE_*` is the one that matters: set those keys and
people can buy credits by card. The `X402_*` and chain variables configure
stablecoin settlement, which funds credits — the way in is still MCP with a
token.

| Variable | What it does |
|---|---|
| `X402_PAY_TO` | Your wallet address — receives x402 payments |
| `CRYPTO_TOPUP` | `true` to offer paying in crypto: the USDC card on /wallet and the address in `wallet_balance`. Off by default — accepting x402 payments is separate and unaffected |
| `X402_NETWORK` · `X402_ASSETS` | Chain and accepted tokens |
| `X402_SERVERS` | Other MCP servers this instance may pay, as `name=url` — read by the outbound client, which no tool currently exposes |
| `CDP_API_KEY_ID` · `CDP_API_KEY_SECRET` | Coinbase facilitator credentials |
| `STRIPE_SECRET_KEY` · `STRIPE_PUBLISHABLE_KEY` · `STRIPE_WEBHOOK_SECRET` | Card top-ups for credits |
| `BASE_RPC_URL` · `TRADE_CHAIN` · `TRADE_RPC_URL` | On-chain reads |

## Prices and limits

Every published price can be overridden with `CREDIT_COST_<OPERATION>` — for
example `CREDIT_COST_SEARCH=2` or `CREDIT_COST_IMAGE=20`. The operation names
are the ones on [/pricing](https://micro.mu/pricing), which is generated from
the same catalogue the charges come from, so that page is the list rather than
twenty-five rows here.

| Variable | Default | What it does |
|---|---|---|
| `DAILY_QUOTA` · `FREE_DAILY_QUOTA` | — | Credits granted per day, paid and free tiers |
| `POST_LIMIT_PER_HOUR` · `NEW_POST_LIMIT_PER_HOUR` | — | Posting rate limit, and the tighter one for new accounts |
| `VIDEO_SEARCH_PER_HOUR` | 20 | YouTube searches one account may run per hour |
| `VIDEO_SEARCH_PER_DAY` | 80 | YouTube searches this instance may run per day, kept under the API's own quota |
| `SIGNUP_MAX_PER_IP` · `SIGNUP_WINDOW_HOURS` | — | Signups allowed per IP, and the window |
| `STATUS_STREAM_LIMIT` · `STATUS_STREAM_LIMIT_PER_USER` | — | How much of the status stream is kept |
| `X402_FACILITATOR_URL` | Coinbase | x402 facilitator to settle through |

## Runtime

| Variable | Default | What it does |
|---|---|---|
| `MU_REGISTRY` | in-process | `mdns` puts services on the local network — note it *announces* every service this process hosts |
| `MU_ADVERTISE` | loopback | Address to advertise when the registry is networked |
| `MU_USE_SQLITE` | — | SQLite with FTS5 for the search index, instead of the file store |
| `MU_SOURCE_DIR` | — | Source tree, for the admin source viewer |
| `MCP_GATEWAY_ADDR` | — | Run go-micro's MCP gateway on its own port |
| `PUBLIC_URL` · `APP_URL` | — | Public origin, when it can't be derived |
| `TOR_ONION` | — | Onion address, shown in the footer |
| `NOTES` | on | Mu posts its own story to its own blog on a low cadence; `off` disables |

## CLI

| Variable | What it does |
|---|---|
| `MU_TOKEN` | Personal Access Token |
| `MU_URL` | Instance to talk to — defaults to the hosted one |
| `MU_NO_COLOR` | Disable colour output |
