# Hosting and maintenance

For an always-on server or your own domain. For a local installation,
start with [Install](INSTALL.md).

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
setting `S3_*` moves stored file bytes to an object store (see the [configuration reference](CONFIGURATION.md#object-storage-and-generation-policy)).

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
