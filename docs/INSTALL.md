# Install

Self-hosting Mu gives you complete control over your data and platform.

## Requirements

- **Go 1.25+** — [golang.org/dl](https://golang.org/dl/)
- **Linux/macOS** — Windows via WSL2
- A server with a public IP, if you want inbound mail

## Quick Start

```bash
# Clone the repository
git clone https://github.com/micro/mu.git
cd mu

# Build and run the server
go build -o mu .
./mu --serve
```

`--serve` is the switch between the two things the binary is: with it you get
the server, without it the same binary is the CLI (`mu news_list`, `mu agent
"..."` — see [Help](/help)). Forget it and you get `--serve not set`.

Mu runs on **port 8080** by default. Visit `http://localhost:8080`, create the
first account — it becomes admin — and pick an AI provider.

## Configuration

Nothing is required to start. Each key below switches on the feature next to it,
and every one of them can also be set at `/admin/env` in the browser once you are
admin, so the environment is for the things you want fixed at deploy time.

```bash
# An AI provider — one of these, for the agent, chat and summaries
export ANTHROPIC_API_KEY="your-key"   # Claude, from console.anthropic.com
# export ATLAS_API_KEY="your-key"     # Atlas Cloud (DeepSeek, Qwen), also images
# export OPENAI_BASE_URL="http://localhost:11434/v1"  # Ollama or any compatible endpoint

# Video
export YOUTUBE_API_KEY="your-key"  # Google Cloud Console

# Places — falls back to OpenStreetMap without it
export GOOGLE_API_KEY="your-key"   # enable Places API (New) and the Routes API

# Web search
export BRAVE_API_KEY="your-key"

# Card top-ups for credits
# export STRIPE_SECRET_KEY="sk_live_..."
# export STRIPE_PUBLISHABLE_KEY="pk_live_..."
# export STRIPE_WEBHOOK_SECRET="whsec_..."
```

Mu also reads a dotenv file at startup: `$MU_ENV_FILE`, then `~/.env`, then
`~/.mu/.env` — the first that exists wins.

Every setting the code reads is listed under Configuration reference below.

## Production Deployment

### Using systemd

Create `/etc/systemd/system/mu.service`:

```ini
[Unit]
Description=Mu Personal AI Platform
After=network.target

[Service]
Type=simple
User=mu
WorkingDirectory=/home/mu
ExecStart=/home/mu/mu --serve
Restart=always
RestartSec=5
EnvironmentFile=/home/mu/.env

[Install]
WantedBy=multi-user.target
```

Then:

```bash
sudo systemctl daemon-reload
sudo systemctl enable mu
sudo systemctl start mu
```

### Using Docker

The repository ships a `Dockerfile` and a `docker-compose.yml`, so there is
nothing to write:

```bash
git clone https://github.com/micro/mu && cd mu
docker compose up
```

The compose file mounts a named volume at `/data` and sets `HOME=/data`, which
is where everything under `~/.mu` lands — keep that volume and you keep your
instance. Uncomment the provider you want in `docker-compose.yml`, or pass keys
with `--env-file`.

By hand, without compose:

```bash
docker build -t mu .
docker run -p 8080:8080 -v mu-data:/data --env-file .env mu
```

### Reverse Proxy (nginx)

```nginx
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Use [Let's Encrypt](https://letsencrypt.org/) for free SSL certificates with Certbot.

## Mail

To send and receive as your own domain:

1. **MX record** pointing at your server.
2. **Port 25** inbound — or `MAIL_PORT=2525` for testing.
3. **DKIM keys**, so your mail is signed and not treated as spam:

```bash
./scripts/generate-dkim-keys.sh
```

That prints a `DKIM_PRIVATE_KEY` for your environment and a TXT record to add at
`<selector>._domainkey.<your-domain>`, where the selector is `MAIL_SELECTOR`
(default `default`).

4. **SPF** — a TXT record at your domain authorising your server to send.

Set `MAIL_DOMAIN` to the domain and restart. `mail_send` is account-only: an
unauthenticated caller can never send, so a paying agent cannot spend your
domain's reputation.

## Discord

1. Create an application at [discord.com/developers/applications](https://discord.com/developers/applications).
2. **Bot → Reset Token**, copy it.
3. Enable **Message Content Intent** under Privileged Gateway Intents.
4. Paste it at `/admin/env` under `DISCORD_BOT_TOKEN`, or set the env var.
5. Invite the bot to your server with the OAuth2 URL generator — scopes `bot`
   and `applications.commands`.

Slash commands: `/agent`, `/news`, `/markets`, `/weather`, `/mail`, `/social`,
`/blog`, `/video`, `/search`, `/apps`, `/balance`, `/usage`.

## Telegram

1. Message [@BotFather](https://t.me/BotFather), send `/newbot`, follow the
   prompts and copy the token.
2. Paste it at `/admin/env` under `TELEGRAM_BOT_TOKEN`, or set the env var.

Commands: `/start`, `/agent` (also `/ask`, `/mu`), `/news`, `/markets`,
`/weather`, `/usage`.

## Taking payments

Callers pay in credits, prepaid against an account. Set the `STRIPE_*` keys to
let people buy them by card; without those keys your instance runs with no
metering, which is usually what you want for one you run for yourself.

Costs are per operation and are set in code — see the cost block in
`internal/quota/quota.go` for what is charged and why.

## Federation (optional)

Set `MU_DOMAIN` to your public domain and blog posts federate over ActivityPub —
remote servers resolve your users at `/.well-known/webfinger` and actor URLs
under that domain. It must match the domain you actually serve on.

## Tor Hidden Service (Optional)

Mu can be accessed as a Tor hidden service (.onion) for anonymous access.

### 1. Install Tor

```bash
sudo apt install tor
```

### 2. Configure the hidden service

Add to `/etc/tor/torrc`:

```
HiddenServiceDir /var/lib/tor/mu/
HiddenServicePort 80 127.0.0.1:8080
```

Restart Tor and get your .onion address:

```bash
sudo systemctl restart tor
sudo cat /var/lib/tor/mu/hostname
```

### 3. Configure passkeys for .onion access

If you use passkeys, add the .onion origin so WebAuthn works on both domains:

```bash
export PASSKEY_EXTRA_ORIGINS="http://your-onion-address.onion"
```

Note: Passkeys registered on `your-instance` won't work on the `.onion` address (WebAuthn spec limitation). Users can register separate passkeys for each origin, or use password login over Tor.

### 4. Nginx for .onion (optional)

If using nginx, add a server block for the .onion address:

```nginx
server {
    listen 80;
    server_name your-onion-address.onion;

    location / {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

No TLS needed — Tor provides end-to-end encryption for .onion addresses.

## Data Storage

Everything is under `~/.mu/`:

```
~/.mu/
├── data/            # accounts, sessions, posts, feeds, the search index,
│   │                # settings.json, cached cards — one file per thing
│   └── files/       # bytes stored by the files service
├── store/           # internal service state
├── keys/            # encryption key, DKIM key, the CLI's wallet seed
└── .env             # optional dotenv, read at startup
```

Back up that directory and you have backed up the instance. It is plain JSON on
disk, so it is greppable and diffable; `MU_USE_SQLITE=1` moves just the search
index into `~/.mu/data/index.db`, and setting `S3_*` moves stored file bytes to
an object store (see Configuration reference below).

In Docker, `HOME` is `/data`, so this tree is `/data/.mu` on the mounted volume.

## Updating

```bash
cd mu
git pull origin main
go build -o mu .
sudo systemctl restart mu
```

## Troubleshooting

**Port already in use:**
```bash
# Find what's using port 8080
lsof -i :8080
```

**Check logs:**
```bash
journalctl -u mu -f
```

**Run without building:**
```bash
go run . --serve
```

## Configuration reference

Every variable below is one the code actually reads — `TestEveryConfigVarIsDocumented`
checks this page against every `settings.Get` and `os.Getenv` in the source, in both
directions, so it can neither fall behind nor accumulate settings that no longer
exist. Any of them can also be set at `/admin/env` in the browser.

### Core

| Variable | Default | What it does |
|---|---|---|
| `ADMIN` / `MU_ADMIN` | first account | Who is admin — comma-separated ids, usernames or emails |
| `MU_DOMAIN` | `localhost` | Public domain. Used for the OAuth issuer an MCP client discovers, Stripe returns, ActivityPub actor URLs and mail. Set this if you run behind a proxy |
| `MU_ENV_FILE` | `~/.env`, then `~/.mu/.env` | A dotenv file read at startup; the first that exists wins. Settings saved at `/admin/env` go to `~/.mu/data/settings.json` instead |
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
| `ANTHROPIC_MODEL` · `ANTHROPIC_PREMIUM_MODEL` | Override the default models |
| `ATLAS_API_KEY` | Atlas Cloud (DeepSeek, Qwen) — also image generation |
| `IMAGE_MODEL` | Override the image model |
| `OPENAI_BASE_URL` · `OPENAI_API_KEY` | Any OpenAI-compatible endpoint — Ollama, vLLM, llama.cpp |
| `AGENT_NATIVE` | `off` falls back to the hand-rolled planner |
| `AGENT_NATIVE_STREAM` | `off` forces the streaming UI onto the planner |

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
| `TWILIO_ACCOUNT_SID` · `TWILIO_AUTH_TOKEN` | — | Twilio credentials. The auth token also verifies inbound webhooks, so an instance without it cannot receive |
| `TWILIO_FROM` | — | The numbers texts are sent from and received on, in E.164 (`+447700900123`), comma-separated. **One per country you serve.** The sender is chosen to match the destination — a US long code texting a UK handset is filtered by UK carriers, and a UK number texting a US handset is blocked outright, so a country with no number of its own is refused rather than sent from the wrong one |
| `TWILIO_MESSAGING_SERVICE_SID` | — | A Twilio Messaging Service to send through instead of picking a number here. With **Geomatch** enabled it chooses the sender whose country matches the handset, which is the same rule applied by the party that knows which of your numbers are registered for what. Set `TWILIO_FROM` as well so the page can say what a reply will come from |
| `SMS_COUNTRIES` | `1,44,353,33,49,34,39,31` | Country codes this instance will text, comma-separated. An allowlist rather than a blocklist: a text to a premium range can cost fifty times what one to a mobile does, and those ranges are where revenue-share fraud lives |
| `SMS_DAILY_LIMIT` | `20` | Messages one account may send in a day, on top of the per-message price. **Set it to `0` to stop sending entirely** — that is the kill switch, and it is the same setting rather than a second one because an operator reaching for it is in a hurry |
| `SMS_NEW_ACCOUNT_LIMIT` | `3` | The same cap for an account less than a day old. Signing up is free and takes a minute, so this is the only thing between a script and the full allowance |
| `SMS_KNOWN_ONLY` | off | Restrict sending to numbers the caller already knows — someone in their contacts, a number they verified as their own, or one that texted them first. Off, because `contacts_add` takes any number and defeats it in one call, and because it stopped an agent doing the ordinary thing. On, it is a real brake for an instance that wants one |
| `SMS_DEFAULT_COUNTRY` | — | Country code assumed for a number written without one. Unset, a number with no `+` is refused rather than guessed |

Senders have to be registered before they will deliver. In the **US**, an
unregistered long code is blocked by every major carrier: either a toll-free
number with toll-free verification (free, reviewed in days, two-way, the
shortest path for low volume) or a 10DLC long code with a brand and campaign
registered through The Campaign Registry. In the **UK**, use a virtual mobile
number (`+447…`) rather than an alphanumeric sender ID — an alphanumeric sender
cannot receive, which means no replies and no way for anyone to text STOP, and
US carriers reject alphanumeric senders outright.

Point each number's inbound webhook at `https://<your domain>/sms/webhook`. The
request is verified against `TWILIO_AUTH_TOKEN`, so nothing else needs opening
up, and `MU_DOMAIN` has to match what Twilio calls or the signature will not
check out.

### File storage

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

### Mail

| Variable | Default | What it does |
|---|---|---|
| `MAIL_DOMAIN` | — | The domain you send and receive as |
| `MAIL_PORT` | `2525` | SMTP listener — `25` in production |
| `MAIL_SELECTOR` | `default` | DKIM selector, the `<selector>._domainkey` DNS record |
| `DKIM_PRIVATE_KEY` | — | DKIM signing key |

DNS records are above.

### Channels

| Variable | What it does |
|---|---|
| `DISCORD_BOT_TOKEN` | Discord bot |
| `TELEGRAM_BOT_TOKEN` | Telegram bot |
| `WHATSAPP_TOKEN` · `WHATSAPP_PHONE_ID` · `WHATSAPP_APP_SECRET` · `WHATSAPP_VERIFY_TOKEN` | WhatsApp Business API |

### Sign-in

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

### Payments

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

The full list is on [/pricing](https://micro.mu/pricing), which renders from
that same file, so this page does not repeat twenty-six rows.

| Variable | Default | What it does |
|---|---|---|
| `DAILY_QUOTA` · `FREE_DAILY_QUOTA` | — | Credits granted per day, paid and free tiers |
| `POST_LIMIT_PER_HOUR` · `NEW_POST_LIMIT_PER_HOUR` | — | Posting rate limit, and the tighter one for new accounts |
| `VIDEO_SEARCH_PER_HOUR` | 20 | YouTube searches one account may run per hour |
| `VIDEO_SEARCH_PER_DAY` | 80 | YouTube searches this instance may run per day, kept under the API's own quota |
| `SIGNUP_MAX_PER_IP` · `SIGNUP_WINDOW_HOURS` | — | Signups allowed per IP, and the window |
| `GUEST_MAX_PER_IP` · `GUEST_WINDOW_MINUTES` | 120 · 60 | Free tool calls an unauthenticated caller may make per IP. Credits price what a call costs; this is what stops a loop, since a free call is charged nothing |
| `X402_FACILITATOR_URL` | Coinbase | x402 facilitator to settle through |

### Runtime

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

### CLI

| Variable | What it does |
|---|---|
| `MU_TOKEN` | Personal Access Token |
| `MU_URL` | Instance to talk to — defaults to the hosted one |
| `MU_NO_COLOR` | Disable colour output |
