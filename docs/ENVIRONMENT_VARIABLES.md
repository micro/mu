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
| `MU_DOMAIN` | `localhost` | Public domain. Used for x402 resource URLs, Stripe returns, ActivityPub actor URLs and mail. Set this if you run behind a proxy |
| `MU_ENV_FILE` | `~/.mu/env` | Where stored settings live |
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
| `GOOGLE_API_KEY` | `places_search`, `places_nearby` — open-data fallback without it |

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
| `GOOGLE_CLIENT_ID` · `GOOGLE_CLIENT_SECRET` | Google sign-in |
| `GOOGLE_REDIRECT_URI` | Defaults to `<your-origin>/oauth2/callback` |
| `PASSKEY_ORIGIN` · `PASSKEY_RP_ID` · `PASSKEY_EXTRA_ORIGINS` | WebAuthn — derived from the request when unset |

## Payments

`X402_PAY_TO` is the one that matters: set it to your wallet address and agents
pay you per call.

| Variable | What it does |
|---|---|
| `X402_PAY_TO` | Your wallet address — receives x402 payments |
| `X402_NETWORK` · `X402_ASSETS` | Chain and accepted tokens |
| `X402_SERVERS` | Other MCP servers the `pay` tool may call |
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
