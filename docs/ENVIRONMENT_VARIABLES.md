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

## Mail Configuration

```bash
# SMTP server port for receiving mail
export MAIL_PORT="2525"              # Default: 2525 (use 25 for production)

# Mail domain for email addresses
export MAIL_DOMAIN="yourdomain.com"  # Default: localhost

# DKIM signing selector (requires keys in ~/.mu/keys/dkim.key)
export MAIL_SELECTOR="default"       # Default: default
```

**Notes:**
- SMTP server always runs automatically
- Mu sends external emails directly to recipient mail servers (no relay needed)
- DKIM signing enables automatically if keys exist at `~/.mu/keys/dkim.key`
- Mail access is restricted to admins and members only

## Example Usage

### Development (Local Testing)

```bash
# SMTP server on port 2525
export MAIL_PORT="2525"
export MAIL_DOMAIN="localhost"

./mu --serve --address :8080
```

### Production

```bash
# SMTP server on standard port 25
export MAIL_PORT="25"

# Your domain and DKIM configuration
export MAIL_DOMAIN="yourdomain.com"
export MAIL_SELECTOR="default"

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
| `MAIL_PORT` | `2525` | Port for SMTP server (use 25 for production) |
| `MAIL_DOMAIN` | `localhost` | Your domain for email addresses |
| `MAIL_SELECTOR` | `default` | DKIM selector for DNS lookup |

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

# Mail (SMTP server always runs)
MAIL_PORT=2525
MAIL_DOMAIN=yourdomain.com
MAIL_SELECTOR=default
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

