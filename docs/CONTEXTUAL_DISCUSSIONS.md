# Contextual Discussions & Chat Improvements

This document describes the contextual discussion system and recent improvements to the chat functionality.

## Overview

The contextual discussion feature allows users to have AI-assisted conversations about specific content (news articles, blog posts, videos) with full context awareness. The system uses RAG (Retrieval-Augmented Generation) with hierarchical search to provide accurate, context-aware answers.

## Architecture

### Chat Rooms

Each piece of content (article, post, video) can have its own discussion room accessed via:
- `/chat?id=news_{itemID}` - News article discussion
- `/chat?id=post_{itemID}` - Blog post discussion  
- `/chat?id=video_{itemID}` - Video discussion

Rooms are ephemeral (in-memory only) and maintain:
- Last 20 messages in server memory
- Full message history in client sessionStorage
- Real-time WebSocket communication

### Content Indexing

All content is indexed with vector embeddings for semantic search:

1. **News Articles**: 
   - Title + description + full article content + HN comments (when available)
   - Metadata: URL, category, published date, image
   
2. **Blog Posts**: 
   - Title + full post content
   - Metadata: Author, posted date
   
3. **Videos**: 
   - Title + description
   - Metadata: URL, channel, published date, thumbnail

### 3-Stage Hierarchical RAG

When a user asks a question in a chat room:

#### Stage 1: Broad Search
- Performs 2 searches (up to 20 candidates total):
  1. Title + question (finds topic-related content)
  2. Question only (finds directly relevant content)
- Creates short snippets (title + 150 chars) for each candidate
- Deduplicates results

#### Stage 2: LLM Reranking
- If >5 candidates, asks LLM to pick the 3-5 most relevant
- Sends only snippets (not full content) to save tokens
- LLM responds with document numbers: "1,3,5"
- Falls back to first 5 if reranking fails

#### Stage 3: Full Context
- Retrieves full content (up to 1000 chars) for selected documents
- Combines with room context (up to 2000 chars)
- Sends to LLM for final answer generation

**Token Efficiency**: Reduced from ~12,000 to ~7,000 chars while improving relevance.

### System Prompt Design

The LLM receives structured context with explicit instructions:

```
[PRIMARY TOPIC] Discussion topic: {room.Title}. {room.Summary}
[Source 1] {title}: {content}
[Source 2] {title}: {content}
...

Instructions:
1. Read the PRIMARY TOPIC carefully - this is what the user is asking about
2. Answer using information from the sources above
3. Search sources for specific information requested
4. If sources don't have the answer, admit it
5. NEVER make up information or answer about unrelated topics
```

This prevents hallucination and keeps answers focused on the discussion topic.

## Content Enhancement

### Article Content Extraction

Articles are fetched and parsed for full content using multiple strategies:

**Common Selectors** (in priority order):
- `.ArticleBody-articleBody` (CNBC)
- `article` (Generic HTML5)
- `.article-body`, `.post-content` (Common patterns)
- `.entry-content` (WordPress)
- `[itemprop='articleBody']` (Schema.org)
- `.story-body` (BBC-style)
- `main article` (Semantic HTML)

**Fallback**: Extracts `<p>` tags from body (excluding nav/footer/aside), limited to 2000 chars.

### HackerNews Comment Integration

For HN articles (`news.ycombinator.com/item?id=`):

1. Extracts story ID from URL
2. Fetches via HN Firebase API: `https://hacker-news.firebaseio.com/v0/item/{id}.json`
3. Retrieves top 10 comments
4. Formats as: `[username]: comment text`
5. Appends to indexed content as `Comments` field

**Rate Limiting**: 50ms delay between comment requests.

### Extensibility

The `Metadata.Comments` field is generic and can support other sources:

```go
// Future: Add Reddit, forums, etc.
if strings.Contains(uri, "reddit.com") {
    g.Comments = fetchRedditComments(uri)
}
```

## Vector Search Optimization

### Parallel Search

Search uses 4 goroutines to process the index in parallel:
- Splits index into 4 chunks
- Each worker calculates cosine similarity for its chunk
- Results collected via channel and merged
- ~4x faster on multi-core systems

### Embedding Model

- **Provider**: Ollama (local)
- **Model**: `nomic-embed-text`
- **Endpoint**: `http://localhost:11434/api/embeddings`
- **Indexed**: Title + first 500 chars (full content stored but not embedded)
- **Threshold**: 0.3 cosine similarity minimum

### Search Algorithm

```go
1. Generate query embedding via Ollama
2. Compare against all indexed entries (parallel)
3. Filter by similarity > 0.3
4. Sort by similarity descending
5. Return top N results
6. Fallback to text matching if embedding fails
```

## UI/UX Features

### Chat Interface

**Regular Chat**:
- Topic-based contexts (Crypto, Tech, Politics, etc.)
- Summaries generated hourly
- Conversation history in sessionStorage
- Topic switching preserves history

**Room Chat**:
- WebSocket real-time communication
- Server broadcasts to all connected clients
- Presence indicators show active users
- Context displayed at top (first sentence only)
- Full context (2000 chars) sent to LLM
- Link to original content

### Markdown Rendering

Supports common markdown patterns:
- `**bold**` → **bold**
- `*italic*` → *italic*
- `` `code` `` → `code`
- ` ```code blocks``` ` → code blocks
- `[link](url)` → links (open in new tab)
- Line breaks preserved

### Discuss Links

All content types have "Discuss" links:
- News articles: In article card
- Blog posts: Below post content
- Videos: On watch page and in thumbnails
- Links to: `/chat?id={type}_{id}`

## Performance Characteristics

### Memory Usage
- In-memory index with embeddings
- Each entry: ~1-2 KB (depending on content length)
- 1000 articles ≈ 1-2 MB RAM
- Room history: 20 messages × avg 200 bytes = ~4 KB per room

### Search Latency
- Embedding generation: ~50-100ms (via Ollama)
- Vector search: ~10-50ms (parallel, depends on index size)
- LLM reranking: ~500-1000ms
- Final answer: ~1-3 seconds

### Scalability
- Current: Linear search, practical up to ~10K entries
- Future: Consider HNSW index for >10K entries
- WebSocket: Handles hundreds of concurrent rooms
- Rate limits: HN API (50ms), Ollama (depends on hardware)

## Configuration

### Environment Variables
- `FANAR_API_KEY` - LLM API key
- Ollama runs on default port 11434

### Tunable Parameters

**In `chat/chat.go`**:
- `room.Summary` max length: 2000 chars
- Search Stage 1: 10 results per query (20 total)
- Search Stage 2: Select 3-5 after reranking
- Search Stage 3: 1000 chars per document
- RAG entry threshold: 0.3 similarity

**In `data/data.go`**:
- Parallel workers: 4 goroutines
- Embedding text limit: 500 chars
- Similarity threshold: 0.3
- Index save interval: on every update

**In `news/news.go`**:
- HN comments fetched: 10 per article
- Article content limit: 2000 chars
- Metadata cache: permanent (by URL hash)

## Troubleshooting

### Question: LLM gives wrong answers
**Check**:
1. Is the article indexed? (Check logs for "Indexing article: {title}")
2. Does room have content? (Check "Room context - Title: ..., Summary length: ...")
3. Are search results relevant? (Check "Search 1/2 returned N results")
4. Did reranking work? (Check "Selected N documents after reranking")

### Question: Duplicate messages in room
**Fixed**: Server history now sole source of truth. SessionStorage used only for reconnection fallback.

### Question: Slow search performance
**Solutions**:
1. Increase parallel workers (currently 4)
2. Implement HNSW index for large datasets
3. Cache embeddings (already done)
4. Use smaller embedding model

### Question: HN comments not appearing
**Check**:
1. Article URL contains "news.ycombinator.com/item?id="
2. Story has comments (kids field non-empty)
3. Rate limit not hit (50ms between requests)
4. Check logs: "Fetched comments for HN story {id}"

## Future Enhancements

1. **Comment Sources**: Reddit, forums, social media
2. **HNSW Index**: For >10K entries
3. **Streaming Responses**: Character-by-character LLM output
4. **Rich Media**: Images, videos in chat
5. **User Annotations**: Highlights, notes on articles
6. **Thread Support**: Nested discussions in rooms
7. **Search Filters**: By date, category, source
8. **Multi-modal**: Image understanding, PDF parsing
