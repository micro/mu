# Data Publishing Architecture

## Current State Analysis

### Problem Summary

The application currently has inconsistent patterns for publishing data to the UI across different packages. While the decoupled approach of having each package manage its own logic is sound, there are several issues:

1. **Inconsistent Caching**: Different packages cache HTML differently
2. **Timestamp Handling**: Varied approaches to displaying relative timestamps
3. **Card Data Format**: No unified structure for home page cards
4. **Update Propagation**: No standard way to notify the home page of content updates
5. **Data Persistence**: Inconsistent use of `data.SaveFile()` and `data.SaveJSON()`

### Current Package Implementations

#### News Package (`news/news.go`)
```go
// Package-level caches
var html string              // Full page HTML
var newsBodyHtml string      // Body HTML without wrapper
var headlinesHtml string     // Headlines HTML
var marketsHtml string       // Markets data HTML
var reminderHtml string      // Reminder HTML
var feed []*Post             // Structured feed data

// Methods exposed to home page
func Headlines() string      // Returns fresh HTML with current timestamps
func Markets() string        // Returns cached marketsHtml
func Reminder() string       // Returns cached reminderHtml
```

**Issues**:
- `Headlines()` generates fresh HTML on every call (recalculates `TimeAgo()` each time)
- `Markets()` and `Reminder()` return pre-cached HTML
- Inconsistent behavior between methods
- No structured data format - returns raw HTML strings

#### Blog Package (`blog/blog.go`)
```go
// Package-level caches
var posts []*Post            // Structured data
var comments []*Comment      // Structured comments
var postsPreviewHtml string  // Cached preview HTML
var postsList string         // Cached full list HTML

// Method exposed to home page
func Preview() string        // Generates fresh HTML with current timestamps

// Cache management
func updateCache()           // Updates HTML caches
func updateCacheUnlocked()   // Internal cache update
```

**Issues**:
- `Preview()` regenerates HTML on every call despite having `blogPreviewHtml` cache
- Has an `updateCache()` function but doesn't use it for the Preview method
- Publishes `blog_updated` event but home page doesn't subscribe to it
- Good: Has structured `Post` type with proper fields

#### Video Package (`video/video.go`)
```go
// Package-level caches
var videos = map[string]Channel{}  // Structured data by category
var latestHtml string               // Cached latest video HTML
var videosHtml string               // Cached saved videos HTML

type Channel struct {
    Videos []*Result `json:"videos"`
    Html   string    `json:"html"`
}

type Result struct {
    ID          string    `json:"id"`
    Type        string    `json:"type"`
    Title       string    `json:"title"`
    Description string    `json:"description"`
    URL         string    `json:"url"`
    Html        string    `json:"html"`
    Published   time.Time `json:"published"`
    Channel     string    `json:"channel,omitempty"`
    Category    string    `json:"category,omitempty"`
}

// Method exposed to home page
func Latest() string  // Generates fresh HTML from cached data
```

**Issues**:
- `Latest()` regenerates HTML on every call
- Has `latestHtml` cache but doesn't use it for the `Latest()` method
- Good: Has well-structured `Result` type
- Good: Saves both structured data and HTML

#### Home Package (`home/home.go`)
```go
type Card struct {
    ID          string
    Title       string
    Column      string // "left" or "right"
    Position    int
    Link        string
    Content     func() string  // Function that returns HTML
    CachedHTML  string         // Cached rendered content
    ContentHash string         // Hash of content for change detection
    UpdatedAt   time.Time      // Last update timestamp
}

var cacheTTL = 2 * time.Minute
```

**Issues**:
- Has a `Card` struct with caching fields but doesn't use them effectively
- Calls `Content()` functions which regenerate HTML every time
- TTL-based cache invalidation (2 minutes) instead of event-driven
- No event subscriptions despite having event system in data package

### Data Package Capabilities

The `data` package already provides good infrastructure:

```go
// Event system
func Subscribe(eventType string) *EventSubscription
func Publish(event Event)

// File persistence
func SaveFile(key, val string) error
func LoadFile(key string) ([]byte, error)
func SaveJSON(key string, val interface{}) error
func LoadJSON(key string, val interface{}) error

// Indexing for RAG/search
func Index(id, entryType, title, content string, metadata map[string]interface{})
```

**Good parts**:
- Event pub/sub system exists
- File save/load helpers
- JSON serialization support

**Unused capabilities**:
- Event system exists but not used for home page updates
- Packages don't publish events when content updates

## Proposed Solution

### 1. Unified Card Data Model

Define a standard card data structure in the `app` package:

```go
// app/app.go
type CardData struct {
    ID        string                 `json:"id"`
    Title     string                 `json:"title"`
    URL       string                 `json:"url"`        // Link for the card
    Image     string                 `json:"image"`      // Optional image URL
    Timestamp time.Time              `json:"timestamp"`  // For sorting and display
    Category  string                 `json:"category"`   // Optional category/tag
    Summary   string                 `json:"summary"`    // Brief description
    Metadata  map[string]interface{} `json:"metadata"`   // Extra data
}

type CardList struct {
    Items     []*CardData `json:"items"`
    UpdatedAt time.Time   `json:"updated_at"`
}
```

### 2. Standardized Publishing Pattern

Each package should follow this pattern:

```go
// 1. Maintain structured data
var items []*Item

// 2. Generate CardData when content updates
func updateCards() {
    cards := &app.CardList{
        Items:     make([]*app.CardData, 0),
        UpdatedAt: time.Now(),
    }
    
    for _, item := range items {
        cards.Items = append(cards.Items, &app.CardData{
            ID:        item.ID,
            Title:     item.Title,
            URL:       item.URL,
            Image:     item.Image,
            Timestamp: item.CreatedAt,
            Category:  item.Category,
            Summary:   item.Description,
        })
    }
    
    // Save to disk
    data.SaveJSON("news_cards.json", cards)
    
    // Publish event
    data.Publish(data.Event{
        Type: "home_feed_updated",
        Data: map[string]interface{}{
            "source": "news",
        },
    })
}

// 3. Provide method to get cards
func GetCards() (*app.CardList, error) {
    var cards app.CardList
    err := data.LoadJSON("news_cards.json", &cards)
    return &cards, err
}

// 4. Still provide HTML rendering for backward compatibility
func Headlines() string {
    cards, err := GetCards()
    if err != nil {
        return ""
    }
    return renderCardsAsHTML(cards.Items[:min(10, len(cards.Items))])
}
```

### 3. Home Page Event Subscription

Update the home package to subscribe to updates:

```go
// home/home.go
func Load() {
    // Subscribe to home feed updates
    feedSub := data.Subscribe("home_feed_updated")
    go func() {
        for event := range feedSub.Chan {
            source, ok := event.Data["source"].(string)
            if ok {
                app.Log("home", "Received update from: %s", source)
                refreshCard(source)
            }
        }
    }()
    
    // ... existing card loading logic
}

func refreshCard(source string) {
    cacheMutex.Lock()
    defer cacheMutex.Unlock()
    
    // Find and refresh the specific card
    for i, card := range Cards {
        if card.ID == source {
            // Re-execute content function to get fresh data
            Cards[i].CachedHTML = card.Content()
            Cards[i].UpdatedAt = time.Now()
            break
        }
    }
}
```

### 4. Consistent Timestamp Rendering

Instead of regenerating HTML on every call, render timestamps on the client side:

```go
// Server side - include timestamp data
func renderCard(data *app.CardData) string {
    return fmt.Sprintf(`
        <div class="card-item" data-timestamp="%d">
            <h3><a href="%s">%s</a></h3>
            <div class="summary">%s</div>
            <div class="timestamp"></div>
        </div>
    `, data.Timestamp.Unix(), data.URL, data.Title, data.Summary)
}
```

```javascript
// Client side - update timestamps periodically
function updateTimestamps() {
    document.querySelectorAll('[data-timestamp]').forEach(el => {
        const timestamp = parseInt(el.dataset.timestamp);
        const timeAgo = calculateTimeAgo(timestamp);
        el.querySelector('.timestamp').textContent = timeAgo;
    });
}

// Update every minute
setInterval(updateTimestamps, 60000);
updateTimestamps(); // Initial call
```

**Alternative**: Keep server-side rendering but cache properly:
- Cache the rendered HTML with timestamps
- Invalidate cache only when content actually changes (not on every read)
- Use short TTL (1-2 minutes) for timestamp freshness

## Implementation Plan

### Phase 1: Define Core Types (Low Risk)
1. Add `CardData` and `CardList` to `app/app.go`
2. Add helper functions for rendering cards
3. No breaking changes - pure additions

### Phase 2: Migrate News Package
1. Implement `GetCards()` method in news
2. Update internal functions to call `updateCards()` on changes
3. Publish events when content updates
4. Keep existing `Headlines()`, `Markets()`, `Reminder()` working

### Phase 3: Migrate Blog Package
1. Implement `GetCards()` method
2. Update `updateCache()` to also generate and save cards
3. Verify existing `Preview()` still works

### Phase 4: Migrate Video Package
1. Implement `GetCards()` method
2. Update video refresh logic to publish events
3. Keep `Latest()` working

### Phase 5: Update Home Package
1. Add event subscriptions for all sources
2. Implement card refresh logic
3. Optimize caching strategy

### Phase 6: Optimize Rendering
1. Decide on client-side vs server-side timestamp rendering
2. Implement chosen approach consistently
3. Add proper cache invalidation

## Benefits

1. **Consistency**: All packages follow the same pattern
2. **Efficiency**: Cache properly, only regenerate when content changes
3. **Maintainability**: Clear structure makes it easier to add new card sources
4. **Flexibility**: Can render cards as HTML, JSON API, or other formats
5. **Real-time Updates**: Event-driven updates instead of polling/TTL
6. **Better UX**: More responsive home page with instant updates

## Risks and Mitigations

### Risk: Breaking Changes
**Mitigation**: Keep all existing methods working during migration. Only remove old code after everything is proven to work.

### Risk: Event System Overhead
**Mitigation**: Events are already buffered (channel size 10). In practice, updates are infrequent.

### Risk: Cache Complexity
**Mitigation**: Start simple - just save CardList JSON files. Optimize later if needed.

### Risk: Timestamp Drift
**Mitigation**: 
- Option A: Client-side rendering eliminates drift
- Option B: Short TTL (1-2 min) keeps server-rendered timestamps fresh enough

## Alternative: Simpler Fix

If the full solution seems too complex, a minimal fix would be:

1. **Standardize caching behavior**: Make all `Preview()`, `Headlines()`, `Latest()` methods use their caches consistently
2. **Fix blog.Preview()**: Use the `blogPreviewHtml` cache instead of regenerating
3. **Fix news.Headlines()**: Cache the generated HTML instead of regenerating
4. **Fix video.Latest()**: Use the `latestHtml` cache
5. **Add cache refresh on updates**: Call update functions when data changes
6. **Standardize timestamps**: Either all client-side or all server-side with TTL

This would fix the immediate inefficiency without restructuring the architecture.

## Recommendation

**Start with the simpler fix** to address the immediate pain points:
- Inconsistent caching is causing unnecessary HTML regeneration
- Timestamps are being recalculated on every page load

**Then consider the full solution** if you want:
- Better structure for future features
- API endpoints returning JSON
- Real-time updates without page refresh
- Better separation of data and presentation

The current architecture isn't fundamentally broken - it just needs consistent application of its own patterns.
