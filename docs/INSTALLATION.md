# Installation

Self-hosting Mu gives you complete control over your data and platform.

## Requirements

- **Go 1.21+** - [golang.org/dl](https://golang.org/dl/)
- **Linux/macOS** - Windows via WSL2
- A server with a public IP (for messaging)

## Quick Start

```bash
# Clone the repository
git clone https://github.com/micro/mu.git
cd mu

# Build and run
go build -o mu .
./mu
```

Mu runs on **port 8080** by default. Visit `http://localhost:8080` to access your instance.

## Configuration

Mu uses environment variables for configuration. Create a `.env` file or export them directly:

```bash
# Required for chat/AI features
export FANAR_API_KEY="your-key"    # Get from api.fanar.qa
# Or use Anthropic:
# export ANTHROPIC_API_KEY="your-key"

# Required for video features
export YOUTUBE_API_KEY="your-key"  # Get from Google Cloud Console

# Optional for Places (falls back to OpenStreetMap without it)
export GOOGLE_API_KEY="your-key"   # Enable Places API (New) in Google Cloud Console

# Optional for card payments
# export STRIPE_SECRET_KEY="sk_live_..."
# export STRIPE_PUBLISHABLE_KEY="pk_live_..."
# export STRIPE_WEBHOOK_SECRET="whsec_..."
```

See [Environment Variables](/docs/environment) for the complete list.

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
ExecStart=/home/mu/mu
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

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o mu .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/mu .
EXPOSE 8080
CMD ["./mu"]
```

Build and run:

```bash
docker build -t mu .
docker run -p 8080:8080 --env-file .env mu
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

## Messaging Setup

To receive and send messages (using SMTP protocol):

1. **DNS Records** - Add MX record pointing to your server
2. **Port 25** - Open inbound port 25 (or set `MAIL_PORT=2525` for testing)
3. **DKIM** - Generate keys for signed messages:

```bash
./scripts/generate-dkim-keys.sh
```

See [Messaging](/docs/messaging) for complete setup.

## Vector Search (Optional)

For semantic search, install Ollama locally:

```bash
curl -fsSL https://ollama.ai/install.sh | sh
ollama pull nomic-embed-text
```

Mu will automatically use embeddings when Ollama is available. See [Vector Search](/docs/vector-search) for details.

## Data Storage

All data is stored in `~/.mu/`:

```
~/.mu/
├── data.db          # SQLite database
├── keys/
│   └── dkim.key     # DKIM private key
└── uploads/         # User uploads
```

Back up this directory to preserve all user data.

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

**Test without building:**
```bash
go run main.go
```
