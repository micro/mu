# Environment Variables

## AI/Chat Configuration

### Fanar API (Primary)

```bash
# Fanar API for chat functionality
export FANAR_API_KEY="your-fanar-api-key"
export FANAR_API_URL="https://api.fanar.ai"  # Default: https://api.fanar.ai
```

### Ollama (Fallback)

```bash
# Used if Fanar API key is not set
export MODEL_NAME="llama3.2"                    # Default: llama3.2
export MODEL_API_URL="http://localhost:11434"   # Default: http://localhost:11434
```

## YouTube Configuration

```bash
# YouTube API key for video functionality
export YOUTUBE_API_KEY="your-youtube-api-key"
```

## Vector Search Configuration

```bash
# Ollama endpoint for semantic vector search (embeddings)
export MODEL_API_URL="http://localhost:11434"   # Default: http://localhost:11434
```

**Note:** Vector search requires Ollama with `nomic-embed-text` model. If unavailable, falls back to keyword search. See [Vector Search Setup](VECTOR_SEARCH.md) for installation.

**TODO:** The Ollama endpoint (`http://localhost:11434`) and embedding model (`nomic-embed-text`) are currently hardcoded in `data/data.go`. Consider making these configurable via `MODEL_API_URL` and a new `EMBEDDING_MODEL` environment variable.

## SMTP Configuration

### SMTP Server (Receiving Mail)

```bash
# Enable SMTP server for receiving mail from the internet
export SMTP_ENABLED="true"               # Default: false (disabled)
export SMTP_SERVER_PORT="2525"           # Default: 2525 (use 25 for production)
export MAIL_DOMAIN="yourdomain.com"      # Default: localhost (or DKIM_DOMAIN)
```

**Note:** SMTP server is **disabled by default**. Set `SMTP_ENABLED=true` to enable mail reception.

### SMTP Client (Sending Mail)

```bash
# Configuration for sending mail to external servers
export SMTP_HOST="smtp.gmail.com"       # Default: localhost
export SMTP_PORT="587"                  # Default: 25
export SMTP_USERNAME="your@email.com"   # Optional
export SMTP_PASSWORD="your-password"    # Optional
```

### DKIM Configuration

```bash
# DKIM signing (optional, requires keys in ~/.mu/keys/dkim.key)
export DKIM_DOMAIN="yourdomain.com"     # Default: localhost
export DKIM_SELECTOR="default"          # Default: default
```

## Example Usage

### Development (Local Testing)

```bash
# Receive on port 2525, send to local SMTP
export SMTP_ENABLED="true"
export SMTP_SERVER_PORT="2525"
export SMTP_HOST="localhost"
export SMTP_PORT="25"
export DKIM_DOMAIN="localhost"

./mu --serve --address :8080
```

### Production

```bash
# Receive on standard port 25
export SMTP_ENABLED="true"
export SMTP_SERVER_PORT="25"

# Send via external relay (e.g., SendGrid, Mailgun)
export SMTP_HOST="smtp.sendgrid.net"
export SMTP_PORT="587"
export SMTP_USERNAME="apikey"
export SMTP_PASSWORD="your-sendgrid-api-key"

# DKIM for your domain
export DKIM_DOMAIN="yourdomain.com"
export DKIM_SELECTOR="default"

./mu --serve --address :8080
```

### Using Gmail SMTP

```bash
# Use Gmail to send outbound mail
export SMTP_HOST="smtp.gmail.com"
export SMTP_PORT="587"
export SMTP_USERNAME="your-email@gmail.com"
export SMTP_PASSWORD="your-app-password"  # Use app-specific password

export DKIM_DOMAIN="yourdomain.com"

./mu --serve --address :8080
```

## All Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FANAR_API_KEY` | - | Fanar API key for chat (required for chat) |
| `FANAR_API_URL` | `https://api.fanar.ai` | Fanar API endpoint |
| `MODEL_NAME` | `llama3.2` | Ollama model name (if Fanar not configured) |
| `MODEL_API_URL` | `http://localhost:11434` | Ollama API endpoint (also used for vector search embeddings) |
| `YOUTUBE_API_KEY` | - | YouTube API key for video functionality |
| `SMTP_ENABLED` | `false` | Enable SMTP server for receiving mail |
| `SMTP_SERVER_PORT` | `2525` | Port for receiving incoming mail |
| `MAIL_DOMAIN` | `localhost` | Domain for email addresses (falls back to `DKIM_DOMAIN`) |
| `SMTP_HOST` | `localhost` | SMTP server for sending outbound mail |
| `SMTP_PORT` | `25` | Port for sending outbound mail |
| `SMTP_USERNAME` | - | Optional SMTP authentication username |
| `SMTP_PASSWORD` | - | Optional SMTP authentication password |
| `DKIM_DOMAIN` | `localhost` | Domain for DKIM signing |
| `DKIM_SELECTOR` | `default` | DKIM selector for DNS lookup |

## .env File (Optional)

Create a `.env` file:

```bash
# AI/Chat
FANAR_API_KEY=your-fanar-api-key
FANAR_API_URL=https://api.fanar.ai
MODEL_NAME=llama3.2
MODEL_API_URL=http://localhost:11434

# YouTube
YOUTUBE_API_KEY=your-youtube-api-key

# SMTP Server (disabled by default)
SMTP_ENABLED=true
SMTP_SERVER_PORT=2525
MAIL_DOMAIN=yourdomain.com

# SMTP Client
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=your-email@gmail.com
SMTP_PASSWORD=your-app-password

# DKIM
DKIM_DOMAIN=yourdomain.com
DKIM_SELECTOR=default
```

Load and run:

```bash
export $(cat .env | xargs) && ./mu --serve --address :8080
```

## Systemd Service Example

```ini
[Unit]
Description=Mu Service
After=network.target

[Service]
Type=simple
User=mu
WorkingDirectory=/opt/mu

# AI/Chat
Environment="FANAR_API_KEY=your-fanar-api-key"
Environment="FANAR_API_URL=https://api.fanar.ai"

# YouTube
Environment="YOUTUBE_API_KEY=your-youtube-api-key"

# SMTP
Environment="SMTP_ENABLED=true"
Environment="SMTP_SERVER_PORT=25"
Environment="MAIL_DOMAIN=yourdomain.com"
Environment="SMTP_HOST=smtp.sendgrid.net"
Environment="SMTP_PORT=587"
Environment="SMTP_USERNAME=apikey"
Environment="SMTP_PASSWORD=your-key"

# DKIM
Environment="DKIM_DOMAIN=yourdomain.com"
Environment="DKIM_SELECTOR=default"

ExecStart=/opt/mu/mu --serve --address :8080
Restart=always

[Install]
WantedBy=multi-user.target
```

## Docker Example

```dockerfile
FROM golang:1.24 AS builder
WORKDIR /app
COPY . .
RUN go build -o mu

FROM debian:bookworm-slim
COPY --from=builder /app/mu /usr/local/bin/
EXPOSE 8080 25
CMD ["mu", "--serve", "--address", ":8080"]
```

Run with environment variables:

```bash
docker run -d \
  -p 8080:8080 \
  -p 25:25 \
  -e FANAR_API_KEY=your-fanar-api-key \
  -e YOUTUBE_API_KEY=your-youtube-api-key \
  -e SMTP_ENABLED=true \
  -e SMTP_SERVER_PORT=25 \
  -e MAIL_DOMAIN=yourdomain.com \
  -e SMTP_HOST=smtp.sendgrid.net \
  -e SMTP_PORT=587 \
  -e SMTP_USERNAME=apikey \
  -e SMTP_PASSWORD=your-key \
  -e DKIM_DOMAIN=yourdomain.com \
  -v ~/.mu:/root/.mu \
  mu:latest
```

## Getting API Keys

### Fanar API
- Sign up at [Fanar AI](https://fanar.ai)
- Get your API key from the dashboard
- Required for chat functionality

### YouTube API
1. Go to [Google Cloud Console](https://console.cloud.google.com)
2. Create a new project
3. Enable YouTube Data API v3
4. Create credentials (API Key)
5. Required for video search/playback

## Feature Requirements

| Feature | Required Environment Variables |
|---------|-------------------------------|
| Chat | `FANAR_API_KEY` or Ollama (`MODEL_NAME`, `MODEL_API_URL`) |
| Vector Search | Ollama with `nomic-embed-text` model (`MODEL_API_URL`) |
| Video | `YOUTUBE_API_KEY` |
| Receive Email | `SMTP_ENABLED=true`, `SMTP_SERVER_PORT` |
| Send External Email | `SMTP_HOST`, `SMTP_PORT` |
| DKIM Signing | Keys in `~/.mu/keys/`, `DKIM_DOMAIN` |

