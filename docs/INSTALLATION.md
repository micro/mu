# Installation

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
"..."` — see [CLI](/docs/cli)). Forget it and you get `--serve not set`.

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

See [Configuration](/docs/environment) for the complete list.

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
an object store (see [Configuration](/docs/environment)).

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
