# Fanar API Performance Improvements

## Current Issues

The Fanar API integration currently experiences slow responses during app development, impacting the user experience in several ways:

1. **App generation delays**: Creating new apps can take 30-60+ seconds
2. **Modification delays**: Iterative improvements require waiting for each LLM call
3. **Rate limiting**: 35 requests/minute shared quota causes blocking
4. **No feedback**: Users wait without progress indication during generation

## Current Architecture

```
User Request → handleDevelop/handleNew → generateAppCode/modifyAppCode 
    → chat.AskLLM → Fanar API (60s timeout) → Response
```

Issues:
- Synchronous blocking on Fanar API calls
- No caching of similar prompts
- No request deduplication
- Rate limiting causes artificial delays
- Long prompts (especially with full code context) are slow

## Proposed Improvements

### 1. Request Caching (Quick Win)

**Problem**: Identical or similar prompts make redundant API calls.

**Solution**: Cache LLM responses based on prompt hash for a limited time.

```go
// Add to apps.go
var (
    llmCache sync.Map // map[string]cachedResponse
)

type cachedResponse struct {
    Code      string
    Timestamp time.Time
}

func getCachedLLMResponse(prompt string) (string, bool) {
    hash := hashPrompt(prompt)
    if val, ok := llmCache.Load(hash); ok {
        cached := val.(cachedResponse)
        // Cache valid for 1 hour
        if time.Since(cached.Timestamp) < time.Hour {
            return cached.Code, true
        }
        llmCache.Delete(hash)
    }
    return "", false
}

func cacheLLMResponse(prompt, code string) {
    hash := hashPrompt(prompt)
    llmCache.Store(hash, cachedResponse{
        Code:      code,
        Timestamp: time.Now(),
    })
}

func hashPrompt(prompt string) string {
    h := sha256.Sum256([]byte(prompt))
    return hex.EncodeToString(h[:])
}
```

**Impact**: 
- ✅ Eliminates redundant API calls for repeated prompts
- ✅ Reduces load on Fanar API
- ✅ Instant responses for cached requests
- ⚠️ Memory usage increases (mitigated by TTL and LRU eviction)

### 2. Async Processing with Real-time Updates (Medium Win)

**Problem**: Users stare at blank screen waiting for generation.

**Solution**: WebSocket-based progress updates during generation.

Current flow:
```
POST /apps/new → 302 Redirect → Poll /apps/{id}/status (every 2s)
```

Improved flow:
```
POST /apps/new → WebSocket connection → Real-time progress updates
    ↓
    "Generating app structure..." (0s)
    "Adding styles..." (15s)
    "Implementing logic..." (30s)
    "Code ready!" (45s)
```

Implementation in `apps/apps.go`:
```go
type GenerationProgress struct {
    AppID   string  `json:"appId"`
    Stage   string  `json:"stage"`
    Percent int     `json:"percent"`
}

var progressChannels sync.Map // map[string]chan GenerationProgress

func generateWithProgress(appID, prompt string) {
    ch := make(chan GenerationProgress, 10)
    progressChannels.Store(appID, ch)
    defer progressChannels.Delete(appID)
    
    ch <- GenerationProgress{AppID: appID, Stage: "Starting generation...", Percent: 0}
    
    // Make API call
    ch <- GenerationProgress{AppID: appID, Stage: "Waiting for LLM response...", Percent: 25}
    code, err := chat.AskLLM(...)
    
    ch <- GenerationProgress{AppID: appID, Stage: "Validating code...", Percent: 75}
    // validation
    
    ch <- GenerationProgress{AppID: appID, Stage: "Complete!", Percent: 100}
}
```

**Impact**:
- ✅ Better user experience (perceived performance)
- ✅ Users know system is working
- ⚠️ Adds complexity (WebSocket management)

### 3. Prompt Optimization (Quick Win)

**Problem**: Sending full app code (5-10KB) on every modification is slow.

**Solution**: Use diff-based prompts for modifications.

Current:
```
System: "Here's the full code: [5KB of HTML]"
User: "Change button color to blue"
```

Optimized:
```
System: "Code summary: Todo app with add/delete features, 150 lines"
User: "Change button color to blue"
Context: "Relevant snippet: <button class='add-btn'>Add</button>"
```

**Impact**:
- ✅ Faster API responses (smaller prompts)
- ✅ Lower token costs
- ⚠️ Requires code analysis to extract relevant context

### 4. Local Model Fallback (High Impact)

**Problem**: Fanar API is slow and rate-limited.

**Solution**: Use local Ollama for development iterations, Fanar for production.

```go
// In chat/model.go
func (m *Model) Generate(prompt *Prompt) (string, error) {
    // Priority: Anthropic > Fanar > Ollama (current)
    // New: Add development mode flag
    
    if isDevelopmentMode() && prompt.Priority == PriorityHigh {
        // Use fast local model for interactive development
        return generateWithOllama(...)
    }
    
    // Use Fanar for production or low-priority tasks
    return generateWithFanar(...)
}

func isDevelopmentMode() bool {
    return os.Getenv("MU_DEV_MODE") == "1"
}
```

**Impact**:
- ✅ Instant responses (no network latency)
- ✅ No rate limiting
- ✅ Free to use
- ⚠️ Requires Ollama installation
- ⚠️ Local model quality may vary

### 5. Streaming Responses (Medium Win)

**Problem**: User waits for entire response before seeing anything.

**Solution**: Stream partial code as it's generated.

```go
func generateAppCodeStreaming(prompt string) (<-chan string, error) {
    ch := make(chan string, 100)
    
    go func() {
        defer close(ch)
        
        // Stream from LLM
        for chunk := range chat.AskLLMStreaming(prompt) {
            ch <- chunk
        }
    }()
    
    return ch, nil
}
```

**Impact**:
- ✅ Faster time-to-first-byte
- ✅ Better perceived performance
- ⚠️ Requires streaming API support
- ⚠️ Partial code may be invalid during streaming

### 6. Request Deduplication (Quick Win)

**Problem**: Multiple users requesting same/similar apps waste API calls.

**Solution**: Deduplicate in-flight requests.

```go
var (
    inflightRequests sync.Map // map[string]*sync.Mutex
)

func generateAppCode(prompt string) (string, error) {
    hash := hashPrompt(prompt)
    
    // Check if this exact request is already in flight
    mu, _ := inflightRequests.LoadOrStore(hash, &sync.Mutex{})
    mu.(*sync.Mutex).Lock()
    defer func() {
        mu.(*sync.Mutex).Unlock()
        inflightRequests.Delete(hash)
    }()
    
    // Check cache first (another request may have completed)
    if code, ok := getCachedLLMResponse(prompt); ok {
        return code, nil
    }
    
    // Make the actual request
    code, err := chat.AskLLM(...)
    if err == nil {
        cacheLLMResponse(prompt, code)
    }
    return code, err
}
```

**Impact**:
- ✅ Prevents duplicate API calls
- ✅ Saves costs
- ⚠️ First request still slow

## Recommended Implementation Order

### Phase 1: Quick Wins (1-2 days)
1. **Request caching** - Immediate 50%+ reduction in API calls for common prompts
2. **Request deduplication** - Prevent wasted parallel requests
3. **Local model fallback flag** - Enable developers to use Ollama

### Phase 2: UX Improvements (3-5 days)
4. **Better progress feedback** - Show generation status without WebSockets (polling is fine)
5. **Prompt optimization** - Reduce prompt size for modifications

### Phase 3: Advanced (optional)
6. **Streaming responses** - Only if Fanar supports it
7. **WebSocket progress** - Only if polling proves insufficient

## Configuration

Add to `.env`:
```bash
# Performance tuning
MU_DEV_MODE=1                    # Use Ollama for development
MU_LLM_CACHE_TTL=3600           # Cache responses for 1 hour
MU_LLM_CACHE_MAX_SIZE=100       # Max cached responses

# Fanar optimization
FANAR_TIMEOUT=30                # Reduce timeout for faster failures
FANAR_MAX_RETRIES=2             # Retry failed requests
```

## Metrics to Track

After implementing improvements, monitor:

1. **Average generation time**: Target < 15s (down from 60s)
2. **Cache hit rate**: Target > 30%
3. **Rate limit hits**: Target < 5% of requests
4. **User completion rate**: % of users who complete app creation

## Conclusion

The **primary performance bottleneck is Fanar API latency**, not iframe execution. The most impactful improvements are:

1. ✅ **Request caching** (quick, high impact)
2. ✅ **Local model fallback** (quick, high impact for dev)
3. ✅ **Better UX feedback** (medium effort, high perceived impact)

These changes can reduce perceived wait time from 60s to < 5s for cached requests and provide instant feedback for development iterations with Ollama.
