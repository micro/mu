# Install

Run your own instance. One Go binary, one data directory.

## Requirements

- **Go 1.26+** — [golang.org/dl](https://golang.org/dl/). `go.mod` says 1.26 and every build here is pinned to it. An older toolchain does not simply work: depending on how Go's toolchain setting is configured it either downloads 1.26 in the middle of your build or refuses outright
- **Linux/macOS** — Windows via WSL2
- A server with a public IP, if you want inbound mail

## Quick Start

```bash
curl -fsSL https://raw.githubusercontent.com/micro/mu/main/install.sh | sh
mu --serve
```

That fetches a built binary. To build it yourself instead — which is the only
way if you want to change anything, and needs the Go version above:

```bash
git clone https://github.com/micro/mu.git
cd mu
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
# Docker, if this instance offers a shell or lets apps run commands. Wants
# rather than Requires: a machine with no Docker should still serve everything
# else, and Requires would refuse to start at all.
#
# The ordering is the part that matters. Without it, mu can win the race on a
# reboot, find no runtime, and — before the probe learned to retry — go on
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
`$VAR` expansion, no command substitution — so this:

```bash
PATH=$PATH:/usr/local/go/bin
```

does not append anything. It sets PATH to the eleven literal characters
`$PATH` followed by `:/usr/local/go/bin`, and the service then has a PATH
containing no real directory at all. Everything that shells out stops working
at once: no `docker`, so no shell service and no apps that run commands.

The same line written `export PATH=...` behaves completely differently, and
not because systemd understands `export` — it cannot parse `export PATH` as a
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
deploy is a few seconds of refused connections — nginx turns those into 502s,
and it looks like "the server takes ages to come back" even when the process
itself starts in well under a second.

Mu already knows how to adopt a socket systemd is holding for it. Give it one:

```ini
# /etc/systemd/system/mu.socket
[Unit]
Description=Mu web socket

[Socket]
ListenStream=8080
# Keep the socket across restarts of mu.service — this is the line that
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
mode it is in on every start — `Serving on systemd-activated socket` or
`Starting server on :8080`.

The other half of a slow restart is the old process leaving rather than the new
one arriving. Shutdown waits for in-flight requests, and an agent run is a model
call, so one chat open when a deploy lands holds it until that answer finishes
(up to ten seconds). Both halves are logged — `Server stopped in …` and
`boot: … ready in …` — so it is worth reading those before changing anything
else.

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

That block is port 80 only, which is where certbot starts. Once it has a
certificate — `sudo certbot --nginx -d your-domain.com` rewrites this file for
you — what you want to end up with is the redirect and the TLS server:

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

Mu terminates no TLS itself. Everything else that needs it — IMAP, submission,
XMPP for clients — goes in the `stream {}` block below, and the two federated
ports are the exceptions that want nothing in front of them at all.

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

### Reading your mail in a mail client

Mu speaks IMAP, so the mail this instance receives can be read in whatever
client is already open — Mail.app, Thunderbird, your phone — and the agent's
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

Mu has no password — sign-in is a passkey or a link — so an access token is what
goes in the password field. That is the app-password pattern, and it has the
property that matters: a client is revoked on its own without touching how you
sign in. The same token is both halves; a client asks twice.

**Outgoing is a separate listener from the MTA.** `MAIL_PORT` is the server that
receives mail from the internet and authenticates nobody, which is what port 25
is for. `SUBMISSION_PORT` is where *you* send from, and it authenticates
everybody: nothing happens on it before AUTH, and the address in `From` must be
one your account owns, so a token is not a way to send as somebody else.

What goes out through it is the same mail the compose form sends — same
allowance, same price, same rules about who you may write to. See
`service/mail/outbound.go`, which is the only way mail leaves an instance.

**Folders are your addresses.** The inbox holds everything. Each plus-address
tag you have received mail at is a folder of its own — mail to `you+research@`
appears in the folder *INBOX/research* — so an agent's mail can be subscribed to
on its own. *Junk* is what the spam filter caught, where you can see it and
disagree with it.

**TLS is the proxy's job.** Nothing in Mu terminates TLS; the web server runs
behind something that does, and IMAP is the same. Bind the listener to loopback
so only the proxy can reach it, and never expose the plaintext port — a token
would cross it in the clear.

```
IMAP_PORT=127.0.0.1:1143
```

nginx does this with the **stream** module, not the mail one. `ngx_mail` speaks
IMAP itself and wants an `auth_http` endpoint to tell it which backend to use
for each user; Mu authenticates its own sessions, so there is nothing for that
endpoint to decide. `ngx_stream` is a TCP proxy with TLS on the front, which is
exactly the missing piece.

```nginx
# At the TOP LEVEL of nginx.conf — a sibling of http {}, not inside it.
# conf.d/*.conf and sites-enabled/* are both included from within http {},
# so a stream block dropped there fails to load.
stream {
    upstream mu_imap {
        server 127.0.0.1:1143;
    }
    upstream mu_submission {
        server 127.0.0.1:1587;
    }

    server {
        # Both, on a host with an AAAA record. A stream block takes no IPv6
        # listener by default, so `listen 993 ssl` alone binds 0.0.0.0 and a
        # client that resolves AAAA finds nothing. The web server does not
        # show this because the packaged default site carries a listen [::]
        # line of its own.
        listen 993 ssl;
        listen [::]:993 ssl;

        # fullchain.pem, not cert.pem. A browser will fetch a missing
        # intermediate and a mail client will not, so half a chain is a site
        # that works in Firefox and fails in Gmail with the same message.
        ssl_certificate     /etc/letsencrypt/live/your-domain.com/fullchain.pem;
        ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;
        ssl_protocols       TLSv1.2 TLSv1.3;

        proxy_pass    mu_imap;

        # Longer than the server's own 30-minute idle timeout. nginx defaults
        # to 10 minutes here, which silently drops every client sitting on
        # IDLE — the mail arrives and nobody is told.
        proxy_timeout 35m;
    }

    # Outgoing, so the client can reply. 465 is implicit TLS, the same as 993:
    # this listener offers no STARTTLS, so a client told to use it on 587 would
    # send the token in the clear believing otherwise.
    server {
        listen 465 ssl;
        listen [::]:465 ssl;

        ssl_certificate     /etc/letsencrypt/live/your-domain.com/fullchain.pem;
        ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;
        ssl_protocols       TLSv1.2 TLSv1.3;

        proxy_pass    mu_submission;
        proxy_timeout 5m;
    }
}
```

The same certificate as the web server. On Debian and Ubuntu the module is a
separate package (`apt install libnginx-mod-stream`); elsewhere nginx needs
`--with-stream --with-stream_ssl_module`, which `nginx -V` will tell you.

### XMPP goes in the same block

`XMPP_PORT` is the same arrangement, one more `server` inside the same
`stream {}`. 5223 is XMPP's implicit-TLS port — the direct-TLS one, the same
idea as 993 and 465 — and it is what a modern client tries first.

```
XMPP_PORT=127.0.0.1:5222
```

```nginx
    upstream mu_xmpp {
        server 127.0.0.1:5222;
    }

    server {
        listen 5223 ssl;
        listen [::]:5223 ssl;

        ssl_certificate     /etc/letsencrypt/live/your-domain.com/fullchain.pem;
        ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;
        ssl_protocols       TLSv1.2 TLSv1.3;

        proxy_pass    mu_xmpp;

        # A chat connection is idle most of the time and closing it is a
        # reconnect and a re-auth. The server's own read deadline is 10
        # minutes; nginx must be longer or it is the one hanging up.
        proxy_timeout 15m;
    }
```

There is no STARTTLS on 5222, for the same reason there is none on 143: the
server does not advertise it, so a client told to use it there would be sending
a token in the clear believing otherwise. Bind it to loopback and let 5223 be
the only way in.

**One DNS record makes it findable.** A client given `you@your-domain.com` has
only the domain to go on, and looks up SRV records before it tries anything.
Without one it guesses the domain itself on 5222, which is not where this is.

```
_xmpps-client._tcp.your-domain.com. 3600 IN SRV 5 0 5223 your-domain.com.
```

`_xmpps-client` is direct TLS (XEP-0368) — the record for 5223. **Do not also
publish `_xmpp-client._tcp` at 5222.** That is the STARTTLS record, 5222 is
bound to loopback, and a record pointing at a closed port is worse than no
record: the client tries it, waits, and reports a timeout rather than telling
anybody what is wrong.

The cost is that a client too old to do direct TLS cannot connect at all. That
is the same trade IMAP makes on 143 and it is the right way round — a client
that cannot do TLS properly should fail to connect rather than succeed in the
clear. Conversations, Dino, Gajim and Monal all do direct TLS.

The target needs an A record, which the domain already has: it is the web
server. And open 5223 on the firewall — see *Check it from somewhere else*
below, which applies here unchanged.

### XMPP federation does not go through nginx

`XMPP_S2S_PORT` is 5269 and it faces the internet directly. Open the port on
the firewall; there is no `server {}` block to add.

The reason is the same one that makes federation deployable at all. A server
connecting here proves which domain it is by dialback, not by its certificate:
it hands over a key, this instance opens its own connection to the domain being
claimed and asks whether the key is theirs, and the answer arrives over a link
to whatever address DNS gave for that domain. So the certificate on 5269 is not
what establishes identity — every federated server skips verifying it, and this
one offers a self-signed certificate generated on first use. Putting nginx in
front would terminate a TLS session whose certificate nobody checks, add a hop,
and make every peer's source address `127.0.0.1`.

**One DNS record, and it is optional.**

```
_xmpp-server._tcp.your-domain.com. 3600 IN SRV 5 0 5269 your-domain.com.
```

Optional because a server with no SRV record to go on falls back to the domain
itself on 5269, and the domain already has an A record — it is the web server.
Publish it anyway if the XMPP host is ever going to be somewhere other than the
web host, which is the whole point of the indirection; it is the same record MX
is for mail. Priority and weight do nothing with one target — the 5 and the 0
match the client record above so the two read the same.

What does matter is that `your-domain.com` — or the SRV target, if you publish
one — resolves to this host on 5269 from the outside. That is where other
servers connect *and* where the dialback verification call comes back to, so a
name that resolves somewhere else fails the handshake rather than the message.

To check the port from somewhere else, open a stream at it. A working listener
answers with its own stream header naming your domain:

```bash
printf "<?xml version='1.0'?><stream:stream xmlns='jabber:server' xmlns:stream='http://etherx.jabber.org/streams' xmlns:db='jabber:server:dialback' to='your-domain.com' version='1.0'>" | nc your-domain.com 5269
```

That checks the port is open, which is not the same as federation working.
For that, **/admin/diagnostics** has a Federation check with a link that dials
`jabber.org` and completes a real dialback handshake — SRV lookup, outbound
dial, and their verification call arriving back here. It needs no account and
no recipient, because dialback does not: proving a domain is a conversation
between two servers, and nobody has to be listening at either end of it.

Outbound 5269 has to be open too, and it is the half that gets forgotten:
dialback means this instance dials *out* to every domain that connects to it.
Egress is usually unrestricted, but a locked-down security group that only
allows 80 and 443 out will accept federated connections and then fail every one
of them at verification, which reads like a broken peer rather than a firewall.

### SSH does not go through nginx

`SHELL_SSH_PORT` is the exception, and it is worth being explicit about because
the instinct is to put everything behind the one proxy.

SSH carries its own transport encryption and does its own host-key
verification. There is no TLS to terminate and no virtual host to pick, so
nginx would be a plain TCP forwarder adding a hop, a second timeout to get
wrong, and a source address that is `127.0.0.1` for every session. Open the
port instead:

```
SHELL_SSH_PORT=2222
```

Wrapping it in `stream { server { listen 2222 ssl; ... } }` is the one thing
that is actively wrong — that offers TLS on the front, and an SSH client does
not speak TLS, so every connection fails at the handshake with a message about
neither protocol.

Pick a port other than 22. That one is the host's own `sshd`, and taking it by
accident locks you out of your own machine.

This is 993, implicit TLS — the port every client offers first. There is no
STARTTLS on 143: the server does not advertise it, so a client asked to use it
there would be sending a token in the clear believing otherwise.

**Check it from somewhere else.** Every test run on the server itself passes
while the port is unreachable from the internet, which is how an afternoon
goes. `ss -lntp` showing `0.0.0.0:993` means nginx is bound, and nothing more.

What the failure looks like tells you where it is. *Connection refused*, at
once, means the packets arrived and nothing was listening — nginx is not up, or
not on that port. *Nothing at all*, until it times out, means they were dropped
before they got there: a firewall. A mail client reports both as the same
unhelpful sentence, so the distinction has to come from `openssl`.

Cloud firewalls are the usual culprit, because they are default-deny and
typically opened for 80, 443 and 22 alone — DigitalOcean's Cloud Firewalls
drop rather than reject, so they produce the timeout above. Check the
provider's rules and the host's own (`ufw status`, `iptables -L INPUT -n`):
having both, with only one of them open, is easy to do.

To check the whole path:

```bash
printf 'a1 LOGIN you TOKEN\r\na2 LOGOUT\r\n' | openssl s_client -quiet -crlf -connect your-domain.com:993
```

`stunnel` and Traefik do the same job if nginx is not what is in front.

Folders cannot be created, renamed or deleted from the client, and a client
cannot upload mail into one. Folders here follow your addresses and your mail,
so there is nothing for those commands to do that would still be true a minute
later.

To check the listener before pointing a real client at it,
[`examples/imap-client`](../examples/imap-client) signs in, lists the folders and
prints the newest messages. It is written against
[emersion/go-imap](https://github.com/emersion/go-imap) rather than anything in
this repo, so it fails the way a real client would.

### Outbound deliverability

By default Mu delivers its own mail: it looks up the recipient's MX and speaks
SMTP to it. That is correct and it is not the hard part. The hard part is the
reputation of the IP the packets came from — a new address with no history, no
feedback loop and no bounce processing gets filed as spam by the large providers
however carefully the message is signed, and nothing in the protocol fixes it
from this end.

So outbound can go through a submission server instead:

```bash
export SMTP_RELAY_HOST="smtp.provider.example"   # :587 assumed
export SMTP_RELAY_USER="apikey"
export SMTP_RELAY_PASS="..."
```

Anything that speaks submission works — this is named for the protocol, not for
a provider. The message is still built here and still signed with your own DKIM
key; the relay is one hop, not a rewrite. STARTTLS is required, because the
credential crosses that connection.

Inbound is unchanged either way: Mu runs its own SMTP server and owns the
mailbox, which is the half that matters.

### Who is allowed to send you mail

This instance does not accept mail from strangers. A message gets in if **any
one** of these is true:

| | Rule |
|---|---|
| 1 | It is a reply to something you sent — `In-Reply-To` or `References` matches a Message-ID this server generated. |
| 2 | You have written to that address before. Recorded automatically on the way out. |
| 3 | The sender's domain is whitelisted — see below. |
| 4 | The sender's address is verified on an account here. Somebody who proved they own a mailbox is not a stranger, whatever their domain. |

Anything else is refused with a `550`, so the sender's own mail server tells
them rather than the message disappearing.

**Building your own whitelist.** Set `MAIL_WHITELIST` to a comma-separated list
of domains:

```
MAIL_WHITELIST=acme.com, partner.co.uk, supplier.example
```

It is live — change it at `/admin/config` and the next message is judged by the new
list, no restart. There is also a built-in list of common company and
infrastructure domains. Consumer domains (`gmail.com`, `outlook.com`,
`hotmail.com`) are deliberately **not** on it: they are where unsolicited mail
comes from, and rule 4 already covers the case that matters — your own users
writing in from a personal address.

There used to be a fifth rule: mail addressed to `support@` and nothing else got
through whatever the sender's domain, because the point of a support address is
hearing from people you have never heard of. That also made it the one address
here that spam could reach, and a per-sender cap does nothing about a thousand
senders. The address, the page and the rule are gone.

## Taking payments

Callers pay in credits, prepaid against an account. Set the `STRIPE_*` keys to
let people buy them by card; without those keys your instance runs with no
metering, which is usually what you want for one you run for yourself.

Costs are per operation and are set in code — see the cost block in
`internal/quota/quota.go` for what is charged and why.

## ActivityPub (optional)

Separate from the XMPP federation above, and a different network: this one
publishes blog posts to Mastodon and the rest of the fediverse.

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
disk, so it is greppable and diffable, with two exceptions: the search index is
SQLite in `~/.mu/data/index.db` (`MU_USE_SQLITE=0` puts it back in JSON), and
setting `S3_*` moves stored file bytes to an object store (see Configuration
reference below).

In Docker, `HOME` is `/data`, so this tree is `/data/.mu` on the mounted volume.

One exception, and it is deliberate: under `go test` the tree is a throwaway
directory in the system temp, not `~/.mu`. Ten packages read the store from
`func init()`, which runs before any test can point `HOME` somewhere safe, so
`go test ./...` used to read and write the instance you actually use. A test
that sets `HOME` itself still gets exactly what it asked for. See
`internal/dir`.

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
exist. Any of them can also be set at `/admin/config` in the browser.

### Core

| Variable | Default | What it does |
|---|---|---|
| `ADMIN` / `MU_ADMIN` | first account | Who is admin — comma-separated ids, usernames or emails |
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
| `ANTHROPIC_MODEL` | Override the default model |
| `AI_PROVIDER` | Optional. Which provider to use when this instance has keys for more than one: `anthropic`, `atlascloud`, `openrouter` or `local`. The words `mu setup` writes — `claude`, `atlas`, `ollama` — are accepted too. Unset, the first key found wins in a fixed order (Anthropic, OpenRouter, local, Atlas), which is fine with one key and arbitrary with two: an instance with a large Atlas balance and a small Anthropic one sent everything to Anthropic because the list said so. Setting this puts the agent, chat and the background work on one provider, each using that provider's own model for the job. It does not override a **named** model — `AGENT_MODEL=deepseek-ai/…` names Atlas whatever this says, because the more specific statement wins. A provider named here with no key is ignored, and said once in the log |
| `AGENT_MODEL` | Optional. The model the **agent** runs on — the tool-calling loop, which is every question anybody asks it. Separate from `ANTHROPIC_MODEL` because it is a different cost decision: the agent makes several model calls per question while a summary makes one. Naming a model also picks its provider, so `deepseek-ai/deepseek-v4-pro-0813` runs the agent on Atlas Cloud even on an instance with an Anthropic key, and `claude-opus-5` puts the hardest reasoning on the loop. Unset, the agent uses the same provider order as everything else: Anthropic, then Atlas, then OpenRouter |
| `GUEST_MODEL` | Optional. The model a **signed-out** visitor's question runs on. Unset, a guest gets the quick end of whichever provider this instance uses and a signed-in account gets the thorough one — a better model spends more tokens and more seconds, and somebody who arrived to find one thing out is waiting for it. Set this to put visitors on something specific, including the same model accounts get |
| `MAIL_FORWARD_KEY` | Generated, not set. The key that signs the unsubscribe link in a forwarded message. Written on first use and kept, because that link may be opened a week later and "this link is no longer valid" is the worst thing to say to somebody trying to stop receiving mail. Listed here so it is recognised rather than deleted |
| `ATLASCLOUD_API_KEY` | Atlas Cloud (DeepSeek, Qwen) — also image generation. `ATLAS_API_KEY` still works |
| `GEMINI_API_KEY` | Google Gemini. Not `GOOGLE_API_KEY`, which this instance uses for Maps and Calendar |
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
| `SMS_VERIFY_INBOUND` | on | Require an arriving message to carry a valid Twilio signature. Turn it **off** if this instance authenticates with an API key, because then there is no account auth token and nothing a signature can be checked against — the cost is that anybody who knows the webhook URL can write into somebody's message history and opt numbers out |
| `SMS_DEFAULT_COUNTRY` | — | Country code assumed for a number written without one. Unset, a number with no `+` is refused rather than guessed |
| `TWILIO_WHATSAPP_FROM` | — | The WhatsApp sender, in E.164 (`+447700900123`). WhatsApp rides the same Twilio account and the same webhook as SMS — point Twilio's WhatsApp sender at `https://<your domain>/whatsapp/twilio` — and this is the one setting that turns it on. One number, not a list: a WhatsApp sender is registered with Meta against a business rather than routed by country, so the matching `TWILIO_FROM` needs does not apply. Unset, WhatsApp is off and nothing offers it |
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
| `MAIL_DAILY_LIMIT` | `50` | Messages one account may send in a day — to somebody here or outside, since there is one price for both. It is `limit_env` on `mail_send` in `quota.json`. Writing to yourself or to your own agent is not a send and does not count against it. A self-hosted instance that wants no ceiling raises it here; one that wants no charge sets `CREDIT_COST_MAIL=0`, and one with no Stripe and no x402 is never charged anyway |
| `MAIL_PORT` | `2525` | SMTP listener — `25` in production, `off` to have none |
| `IMAP_PORT` | `1143` | IMAP listener — `143` in production, `off` to have none. See [Reading your mail in a mail client](#reading-your-mail-in-a-mail-client) |
| `SUBMISSION_PORT` | `1587` | SMTP submission, so a mail client can send — `587` in production, `off` to have none |
| `IMAP_PUBLIC` | — | What `/inbox/imap` tells people to connect to, `host:port`. The listener runs in the clear behind a terminator, so the bound port is usually not the port a client dials; unset, the page offers `993` and names the local port beside it |
| `SUBMISSION_PUBLIC` | — | The same for outgoing. Unset, the page offers `465` |
| `MAIL_SELECTOR` | `default` | DKIM selector, the `<selector>._domainkey` DNS record |
| `DKIM_PRIVATE_KEY` | — | DKIM signing key |
| `SMTP_RELAY_HOST` | — | Hand outbound mail to a submission server instead of delivering it to the recipient's MX. `host` or `host:port`, 587 assumed. See [Outbound deliverability](#outbound-deliverability) |
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

DNS records are above, and [Who is allowed to send you mail](#who-is-allowed-to-send-you-mail) is the whole inbound rule.

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
| `SHELL_SHARED` | Optional, default off. `on` pools machines instead of giving one per account: a fixed set of containers all mounting one volume, with `/work/<account>` per caller and every command running as that account's own Unix user. Worth it on a small box, where one container each means the second caller evicts the first. What it gives up is real and is not files — a caller's directory is `0700` and `/work` is sticky like `/tmp`, so nobody can read, list or delete anybody else's — but the machine is shared: other people's processes are visible in `ps` with their command lines, one caller's heavy build slows everybody on that container, and the process limit contains the host rather than the neighbours. Fine on an instance with one user, not on one with strangers, which is why it is a decision rather than something that happens quietly when memory is short |
| `SHELL_MAX_MACHINES` | Optional, default half the host's memory divided by what one machine takes, minimum 1. How many machines may run at once. Each holds its memory cap whether or not anybody is using it, so a box that fits two cannot host five however cheap a command is. Starting one past the cap stops the idlest machine rather than refusing the caller — the volume is untouched and their next command starts it again |
| `SHELL_NETWORK` | Optional, default `bridge`. `none` gives machines no network at all. The default is on because a machine that cannot fetch a dependency or push a branch cannot do the thing this is for — what the container bounds is the host, not the internet |
| `SHELL_MAX_SECONDS` | Optional, default 600. The longest one command may run, whatever it asked for. A command with no timeout of its own gets 120 |
| `XMPP_PORT` | **On by default at `:5222`**; set it to `off` to close the door. A port to answer XMPP on, so `asim@your.domain` is a chat address as well as a mailbox — one account, one local part, reachable two ways. Conversations, Dino, Gajim and Monal are clients for it. Sign in with your username and an access token as the password, the same credential IMAP and submission take. The agent addresses work unchanged: `agent@your.domain` and `you+research@your.domain` are valid JIDs as well as valid mail addresses, and it is the same agent at the end of them. Nothing in Mu terminates TLS, so bind it to loopback and put the proxy on 5223 — see the nginx `stream {}` section above, which is the same arrangement IMAP and submission use, plus the `_xmpps-client._tcp` SRV record a client needs to find it. Federation is on the separate `XMPP_S2S_PORT` below |
| `XMPP_S2S_PORT` | **On by default at `:5269`**; set it to `off` to keep this instance to itself. The federated port, where other XMPP servers connect so that `asim@your.domain` can message somebody on any Prosody, ejabberd or Openfire deployment, and they can message back. Servers prove which domain they are by dialback (XEP-0220): they hand over a key and this instance opens its own connection to the domain they claim and asks whether the key is theirs, so a server that cannot receive mail at the domain it claims cannot pass. That means two things for DNS. Publish `_xmpp-server._tcp.your.domain` pointing at this host on 5269, or make sure `your.domain` itself resolves to it, because that is where other servers look and where the verification call comes back to. And unlike `XMPP_PORT` this one faces the internet directly rather than through the proxy: STARTTLS is offered with a self-signed certificate generated on first use, which is correct here because dialback and not the certificate is what proves the domain — every federated server skips certificate verification on this port for that reason. Turn it off if you want your own people on XMPP without accepting connections from every other server on the internet |
| `SHELL_SSH_PORT` | Optional, **off by default**. A port to answer SSH on, so somebody with a registered public key can open a shell in their own machine — `ssh -p 2222 you@host`. Mu is the SSH server; no `sshd` runs inside a container and no key is ever put in one, so the session lands in the same box with the same caps, memory, CPU and PID limits as a tool call. Keys only, no passwords, and the username is ignored: which key signed the handshake is what says who you are. There is no default because `22` on the host is the host's own `sshd` and taking it by accident locks you out of your own machine — pick a port and open it deliberately. A shell holds a machine open, so it is the most expensive thing a caller can do; sessions are capped at four hours. Register a key at `/shell` |
| `SHELL_IDLE_MINUTES` | Optional, default 30. How long a machine may sit doing nothing before it is stopped. Stopping is not deleting: the `/work` volume is untouched and the next command starts it again in about a second. This is what bounds the memory of machines nobody is using, which a price on commands would not have — the cost is the idle container rather than the calls |
| `OS_MAPS_KEY` | Optional. Ordnance Survey Data Hub key, for `/maps` — the basemap under anything spatial. Free tier at osdatahub.os.uk. Britain only. Without it the service still serves every tile this instance has already fetched, so a lapsed key degrades to the region you have already used rather than to nothing. Tiles are free to callers; what bounds them is `TILE_FETCH_PER_HOUR` |
| `TILE_FETCH_PER_HOUR` | Optional, default 2000. How many tiles one account may make this instance fetch from Ordnance Survey in an hour. Tiles already held are served without limit and without a session, because serving one again costs nothing — this bounds only what is spent upstream. Raise it to seed a region on purpose |
| `X402_SERVERS` | Other MCP servers this instance may pay, as `name=url` — read by the outbound client, which no tool currently exposes |
| `CDP_API_KEY_ID` · `CDP_API_KEY_SECRET` | Coinbase facilitator credentials |
| `STRIPE_SECRET_KEY` · `STRIPE_PUBLISHABLE_KEY` · `STRIPE_WEBHOOK_SECRET` | Card top-ups for credits. Point the endpoint at `https://<your domain>/stripe/webhook` and subscribe it to `checkout.session.completed`. It is belt and braces rather than the only route: the return from Stripe settles a purchase too, so a webhook that is missing, misconfigured or signed with the wrong secret no longer means the card is charged and nothing happens |
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

The full list is on [/tools](https://micro.mu/tools), which renders from
that same file, so this page does not repeat twenty-six rows.

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
| `NOTES` | on | Mu posts its own story to its own blog on a low cadence; `off` disables |
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
| `S3_BUCKET` | — | Bucket for off-box backups. A snapshot on the same disk survives a bad write and not the disk; this is the copy that survives losing the machine. Later it is also where files and generated images belong, which is why these are named for the storage rather than for the backup |
| `S3_REGION` | `us-east-1` | Region of the bucket |
| `S3_ENDPOINT` | — | For anything that is not AWS — R2, Backblaze, MinIO. Leave empty for AWS |
| `S3_ACCESS_KEY_ID` | — | Access key. Give it write access to this bucket and nothing else: it is on a machine that runs model output |
| `S3_SECRET_ACCESS_KEY` | — | Secret key |
| `S3_PREFIX` | — | Optional path inside the bucket, so one bucket can hold several instances |
| `BACKUP_S3` | `false` | Whether backups are pushed to the bucket above |
