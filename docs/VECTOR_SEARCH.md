# Vector Search Setup

## Installation on DigitalOcean VPS

### 1. Install Ollama

```bash
curl -fsSL https://ollama.com/install.sh | sh
```

### 2. Pull the embedding model

```bash
ollama pull nomic-embed-text
```

This downloads ~274MB. The model will automatically start when needed.

### 3. Verify Ollama is running

```bash
curl http://localhost:11434/api/embeddings -d '{
  "model": "nomic-embed-text",
  "prompt": "test"
}'
```

Should return a JSON with an "embedding" array of 768 floats.

### 4. Restart your mu application

```bash
./mu
```

The app will now automatically:
- Generate embeddings for all indexed content (news, tickers, videos)
- Use semantic vector search for queries
- Fallback to keyword search if Ollama is unavailable

## How it works

- **Indexing**: When news/tickers are indexed, embeddings are generated automatically
- **Search**: Queries are embedded and compared using cosine similarity
- **Performance**: ~50-100ms per embedding on 1-2 CPU cores
- **Fallback**: If Ollama is down, keyword search is used automatically

## Testing

Try asking:
- "what's the bitcoin price" → should find BTC ticker
- "ethereum value" → should find ETH ticker  
- "crypto markets" → should find crypto-related news
- "digital gold" → should find Bitcoin content

## Memory usage

- Ollama idle: ~100MB
- During embedding: +400MB temporarily
- Index with embeddings: ~4KB per entry
