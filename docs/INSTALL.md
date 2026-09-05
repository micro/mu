# Install

Run your own instance. One Go binary, one data directory.

## Requirements

- **Go 1.26+** â€” [golang.org/dl](https://golang.org/dl/). `go.mod` says 1.26 and every build here is pinned to it. An older toolchain does not simply work: depending on how Go's toolchain setting is configured it either downloads 1.26 in the middle of your build or refuses outright
- **Linux/macOS** â€” Windows via WSL2
- A server with a public IP, if you want inbound mail

## Quick Start

```bash
curl -fsSL https://raw.githubusercontent.com/micro/mu/main/install.sh | sh
mu --serve
```

That fetches a built binary. To build it yourself instead â€” which is the only
way if you want to change anything, and needs the Go version above:

```bash
git clone https://github.com/micro/mu.git
cd mu
go build -o mu .
./mu --serve
```

`--serve` is the switch between the two things the binary is: with it you get
the server, without it the same binary is the CLI (`mu news_list`, `mu agent
"..."` â€” see [Help](/help)). Forget it and you get `--serve not set`.

Mu runs on **port 8080** by default. Visit `http://localhost:8080`, create the
first account â€” it becomes admin â€” and pick an AI provider.

## Configuration

Nothing is required to start. Each key below switches on the feature next to it,
and every one of them can also be set at `/admin/config` in the browser once you are
admin, so the environment is for the things you want fixed at deploy time.

```bash
# An AI provider â€” one of these, for the agent, chat and summaries
export ANTHROPIC_API_KEY="your-key"   # Claude, from console.anthropic.com
# export ATLASCLOUD_API_KEY="your-key" # Atlas Cloud (DeepSeek, Qwen), also images
# export GEMINI_API_KEY="your-key"     # Google Gemini
# export OPENROUTER_API_KEY="your-key" # OpenRouter (one key, many models)
# export OPENAI_BASE_URL="http://localhost:11434/v1"  # Ollama or any compatible endpoint
# export OPENAI_MODEL="llama3.2"                      # which model that endpoint serves

# Video
export YOUTUBE_API_KEY="your-key"  # Google Cloud Console

# Places â€” falls back to OpenStreetMap without it
export GOOGLE_API_KEY="your-key"   # enable Places API (New) and the Routes API

# Web search
export BRAVE_API_KEY="your-key"

# Card top-ups for credits
# export STRIPE_SECRET_KEY="sk_live_..."
# export STRIPE_PUBLISHABLE_KEY="pk_live_..."
# export STRIPE_WEBHOOK_SECRET="whsec_..."
```

Mu also reads a dotenv file at startup: `$MU_ENV_FILE`, then `~/.env`, then
`~/.mu/.env` â€” the first that exists wins.

Every setting the code reads is listed under Configuration reference below.

## Production Deployment

### Using systemd

Create `/etc/systemd/system/mu.service`:

```ini
[Unit]
Description=Mu Personal AI Platform
After=network.target
# Docker, if this instance offers a shell or lets apps run commands. Wants
# rather than Requires: a machine with no Docker should still serve everything
# else, and Requires would refuse to start at all.
#
# The ordering is the part that matters. Without it, mu can win the race on a
# reboot, find no runtime, and â€” before the probe learned to retry â€” go on
# saying so for as long as the process lived, on a machine where docker ps
# worked fine.
Wants=docker.service
After=docker.service

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

**`EnvironmentFile` is not a shell, and PATH is the line that catches people.**
systemd reads that file as plain `KEY=value` pairs. There is no `export`, no
`$VAR` expansion, no command substitution â€” so this:

```bash
PATH=$PATH:/usr/local/go/bin
```

does not append anything. It sets PATH to the eleven literal characters
`$PATH` followed by `:/usr/local/go/bin`, and the service then has a PATH
containing no real directory at all. Everything that shells out stops working
at once: no `docker`, so no shell service and no apps that run commands.

The same line written `export PATH=...` behaves completely differently, and
not because systemd understands `export` â€” it cannot parse `export PATH` as a
variable name, so it skips the line, and the service quietly keeps systemd's
own default PATH. It works by being ignored, which is worse than failing,
because removing the word `export` later looks like tidying.

If you need a PATH, write it out in full and put it in the unit rather than
the env file:

```ini
Environment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/local/go/bin
```

To see what the service actually got:

```bash
systemctl show mu -p Environment
sudo -u mu sh -c 'command -v docker'
```

### Restarts without a gap

The unit above drops the listening socket while the binary restarts, so every
deploy is a few seconds of refused connections â€” nginx turns those into 502s,
and it looks like "the server takes ages to come back" even when the process
itself starts in well under a second.

Mu already knows how to adopt a socket systemd is holding for it. Give it one:

```ini
# /etc/systemd/system/mu.socket
[Unit]
Description=Mu web socket

[Socket]
ListenStream=8080
# Keep the socket across restarts of mu.service â€” this is the line that
# turns a refused connection into a queued one.
FileDescriptorName=mu

[Install]
WantedBy=sockets.target
```

and tell the service to use it, by adding to `mu.service`:

```ini
[Unit]
Requires=mu.socket
After=mu.socket

[Service]
# Not needed with a socket, and 5 seconds of nothing on every crash-restart.
RestartSec=1
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now mu.socket
sudo systemctl restart mu
```

The kernel keeps accepting and queueing connections on the held socket while the
process is away, so a restart is latency rather than an error. The log says which
mode it is in on every start â€” `Serving on systemd-activated socket` or
`Starting server on :8080`.

The other half of a slow restart is the old process leaving rather than the new
one arriving. Shutdown waits for in-flight requests, and an agent run is a model
call, so one chat open when a deploy lands holds it until that answer finishes
(up to ten seconds). Both halves are logged â€” `Server stopped in â€¦` and
`boot: â€¦ ready in â€¦` â€” so it is worth reading those before changing anything
else.

### Using Docker

The repository ships a `Dockerfile` and a `docker-compose.yml`, so there is
nothing to write:

```bash
git clone https://github.com/micro/mu && cd mu
docker compose up
```

The compose file mounts a named volume at `/data` and sets `HOME=/data`, which
is where everything under `~/.mu` lands â€” keep that volume and you keep your
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

That block is port 80 only, which is where certbot starts. Once it has a
certificate â€” `sudo certbot --nginx -d your-domain.com` rewrites this file for
you â€” what you want to end up with is the redirect and the TLS server:

```nginx
server {
    listen 80;
    server_name your-domain.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    server_name your-domain.com;

    ssl_certificate     /etc/letsencrypt/live/your-domain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;

    # Apps are served from an opaque origin and some of them are large.
    client_max_body_size 25m;

    location / {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        # The agent answers a question with a model call behind it, and the
        # default 60s cuts long ones off mid-sentence.
        proxy_read_timeout 300s;
    }
}
```

`X-Forwarded-Proto` matters more than it looks: passkeys will not register over
a connection the browser thinks is insecure, and it is that header the server
reads to know it is behind TLS.

Mu terminates no TLS itself. Everything else that needs it â€” IMAP, submission,
XMPP for clients â€” goes in the `stream {}` block below, and the two federated
ports are the exceptions that want nothing in front of them at all.

## Mail

To send and receive as your own domain:

1. **MX record** pointing at your server.
2. **Port 25** inbound â€” or `MAIL_PORT=2525` for testing.
3. **DKIM keys**, so your mail is signed and not treated as spam:

```bash
./scripts/generate-dkim-keys.sh
```

That prints a `DKIM_PRIVATE_KEY` for your environment and a TXT record to add at
`<selector>._domainkey.<your-domain>`, where the selector is `MAIL_SELECTOR`
(default `default`).

4. **SPF** â€” a TXT record at your domain authorising your server to send.

Set `MAIL_DOMAIN` to the domain and restart. `mail_send` is account-only: an
unauthenticated caller can never send, so a paying agent cannot spend your
domain's reputation.

### Reading your mail in a mail client

Mu speaks IMAP, so the mail this instance receives can be read in whatever
client is already open â€” Mail.app, Thunderbird, your phone â€” and the agent's
replies appear in the thread there.

| | |
|---|---|
| Server | your domain |
| Incoming (IMAP) | `IMAP_PORT`, `1143` by default; set it to `143` in production |
| Outgoing (SMTP) | `SUBMISSION_PORT`, `1587` by default; set it to `587` in production |
| Username | your Mu username, or your full address |
| Password | an access token from `/token` |

Signed in, `/inbox/imap` says all of this filled in for the account reading it.
Set `IMAP_PUBLIC` and `SUBMISSION_PUBLIC` to `host:port` if what you put in
front of these listeners answers somewhere other than the defaults below.

Mu has no password â€” sign-in is a passkey or a link â€” so an access token is what
goes in the password field. That is the app-password pattern, and it has the
property that matters: a client is revoked on its own without touching how you
sign in. The same token is both halves; a client asks twice.

**Outgoing is a separate listener from the MTA.** `MAIL_PORT` is the server that
receives mail from the internet and authenticates nobody, which is what port 25
is for. `SUBMISSION_PORT` is where *you* send from, and it authenticates
everybody: nothing happens on it before AUTH, and the address in `From` must be
one your account owns, so a token is not a way to send as somebody else.

What goes out through it is the same mail the compose form sends â€” same
allowance, same price, same rules about who you may write to. See
`service/mail/outbound.go`, which is the only way mail leaves an instance.

**Folders are your addresses.** The inbox holds everything. Each plus-address
tag you have received mail at is a fm|Ûmm¢G§²ÚîÆ­yÑ™¥ÉÍĞÑ¡¥¹œ…¹å‰½‘ä‘½•Ìİ¥Ñ „½µµ…¹µ±¥¹”Ñ½½°¥Ì)ÉÕ¸¥Ğ°…¹€‰¹¼Í•ÉÙ•È½¹™¥ÕÉ•ˆ¥Ì„İ½ÉÍ”™¥ÉÍĞ…¹Íİ•ÈÑ¡…¸„É•ÍÕ±Ğ¸%Ğ)‘½•Ìµ•…¸Ñ¡…Ğ¥˜å½Ô¡…Ù”©ÕÍĞ¥¹ÍÑ…±±•å½ÕÈ½İ¸¥¹ÍÑ…¹”…¹ÑåÁ•)µÔ¹•İÌ±¥ÍÑ€°å½Ô¡…Ù”…±±•Í½µ•‰½‘ä•±Í”ÌƒŠPÍ¼Á½¥¹Ğ¥Ğ…Ğå½ÕÉÌè()‰…Í )µÔ±½¥¸¡ÑÑÁÌè¼½å½ÕÈ¹¡½ÍĞ€€€€€€ŒÍ…Ù•ÌÑ¡”…‘‘É•ÍÌ…¹„Ñ½­•¸°™½È½½)€()Ù•ÉåÑ¡¥¹œ…™Ñ•ÈÑ¡…Ğ½•ÌÑ¼å½ÕÈ¥¹ÍÑ…¹”èÑ¡”Ñ½½°½µµ…¹‘Ì°µÔ…Í­€°)µÔ…•¹Ñ€É•¹Ñ¥¹œÑ½½±Ì½Ù•ÈàĞÀÈ°…¹µÔàĞÀÈ…±±€¸()Q¼¡•¬İ¡…Ğ¥Ì¥¸™½É”°…¹İ¡…Ğ‘•¥‘•¥Ğè()‰…Í (µÔ½¹™¥œ•Ğ)ÕÉ°õ¡ÑÑÁÌè¼½å½ÕÈ¹¡½ÍĞ€ ½¡½µ”½å½Ô¼¹½¹™¥œ½µÔ½½¹™¥œ¹©Í½¸¤)Ñ½­•¸ô¨¨¨)€()Q¡”™½ÕÈÍ½ÕÉ•Ì°ÍÑÉ½¹•ÍĞ™¥ÉÍĞè()ğM½ÕÉ”ğá…µÁ±”ğM½Á”ğ)ğ´´µğ´´µğ´´µğ)ğ€´µÕÉ±€ğµÔ€´µÕÉ°¡ÑÑÁÌè¼½½Ñ¡•È¹¡½ÍĞ¹•İÌ±¥ÍÑ€ğ=¹”½µµ…¹¸5ÕÍĞ½µ”€©‰•™½É”¨Ñ¡”Ñ½½°¹…µ”ƒŠPµÔİ•ˆ™•Ñ €´µÕÉ°ƒŠ™€¥ÌÑ¡”™•Ñ Ñ½½°Ì½İ¸…ÉÕµ•¹Ğğ)ğ5U}UI1€ğ5U}UI0õ¡ÑÑÁÌè¼½½Ñ¡•È¹¡½ÍĞµÔ¹•İÌ±¥ÍÑ€ğ=¹”Í¡•±°ğ)ğ½¹™¥œ™¥±”ğİÉ¥ÑÑ•¸‰äµÔ±½¥¸€ñÕÉ°ù€ğA•Éµ…¹•¹Ğ°Á•ÈÕÍ•Èğ)ğ•™…Õ±Ğğ¡ÑÑÁÌè¼½µ¥É¼¹µÕ€ğ]¡•¸¹½Ñ¡¥¹œ•±Í”Í…åÌğ()ğY…É¥…‰±”ğ]¡…Ğ¥Ğ‘½•Ìğ)ğ´´µğ´´µğ)ğ5U}Q=-9€ğA•ÉÍ½¹…°•ÍÌQ½­•¸¸=Ù•ÉÉ¥‘•ÌÑ¡”Í…Ù•½¹”ğ)ğ5U}UI1€ğ%¹ÍÑ…¹”Ñ¼Ñ…±¬Ñ¼¸=Ù•ÉÉ¥‘•ÌÑ¡”Í…Ù•½¹”ğ)ğ5U}9=}=1=I€ğ¥Í…‰±”½±½ÕÈ½ÕÑÁÕĞğ()Ñ½­•¸‰•±½¹ÌÑ¼Ñ¡”¥¹ÍÑ…¹”Ñ¡…Ğ¥ÍÍÕ•¥Ğ°Í¼¡…¹¥¹œÑ¡”…‘‘É•ÍÌ)İ¥Ñ¡½ÕĞ¡…¹¥¹œÑ¡”Ñ½­•¸•ÑÌå½Ô„€ĞÀÄƒŠPµÔ±½¥¸€ñÕÉ°ù€‘½•Ì‰½Ñ ¸((ŒŒŒ=‰©•ĞÍÑ½É…”…¹•¹•É…Ñ¥½¸Á½±¥ä()ğY…É¥…‰±”ğ•™…Õ±Ğğ]¡…Ğ¥Ğ‘½•Ìğ)ğ´´´´´´´´´µğ´´´´´´´´µğ´´´´´´´´´´´´´µğ)ğLÍ}	U-Q€ğƒŠPğM¡…É•‰Õ­•Ğ™½È‘ÕÉ…‰±”ÍÑ½É…”¸¥±•ÌÕÍ”™¥±•Ì½€ì½™˜µ‰½à‰…­ÕÁÌÕÍ”‰…­ÕÁÌ½€ğ)ğLÍ}I%=9€ğÕÌµ•…ÍĞ´Å€ğI•¥½¸½˜Ñ¡”‰Õ­•Ğğ)ğLÍ}9A=%9Q€ğƒŠPğ½È…¹åÑ¡¥¹œÑ¡…Ğ¥Ì¹½Ğ]LƒŠPHÈ°	…­‰±…é”°5¥¹%<¸1•…Ù”•µÁÑä™½È]Lğ)ğLÍ}MM}-e}%€ğƒŠPğ•ÍÌ­•ä™½ÈÑ¡”‰Õ­•Ğ¸¥±•Ì¹••É•…°İÉ¥Ñ”…¹‘•±•Ñ”…•ÍÌì‰…­ÕÁÌÕÍ”Ñ¡”Í…µ”É•‘•¹Ñ¥…±Ìğ)ğLÍ}MIQ}MM}-e€ğƒŠPğM•É•Ğ­•äğ)ğ	-UA}LÍ€ğ™…±Í•€ğ]¡•Ñ¡•È‰…­ÕÁÌ…É”ÁÕÍ¡•Ñ¼Ñ¡”‰Õ­•Ğ…‰½Ù”Õ¹‘•È‰…­ÕÁÌ½€ğ