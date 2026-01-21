# Mu Tools Design Document

## Overview

A unified tools registry for agent capabilities. Packages register tools that the agent can discover and invoke. Separate from the app/cards system which handles home page previews.

**Key distinction:**
- **Tools** (tools/) - Actions the agent can invoke. Dynamic, LLM-driven.
- **Cards** (app/) - Data previews for home page. Static, display-only.

## Package Interface

```go
// tools/tools.go
package tools

type Tool struct {
    Name        string                                                    // "markets.get_price"
    Description string                                                    // For LLM understanding
    Category    string                                                    // Grouping: "markets", "news"
    Input       map[string]Param                                          // Input schema
    Handler     func(ctx context.Context, params map[string]any) (any, error)
}

type Param struct {
    Type        string   // "string", "number", "bool", "array"
    Description string
    Required    bool
    Enum        []string // Optional: allowed values
}

// Core functions
func Register(tool Tool)
func List() []Tool
func Get(name string) *Tool
func Call(ctx context.Context, name string, params map[string]any) (any, error)

// Context helpers
func WithUser(ctx context.Context, userID string) context.Context
func UserFromContext(ctx context.Context) string
```

## Cards (Separate System)

Cards remain in `app/` package for home page previews:

```go
// app/cards.go
type Card struct {
    ID       string
    Title    string
    Column   string        // "left" or "right"
    Position int
    Link     string
    Content  func() string // HTML preview
}

func RegisterCard(card Card)
func GetCards() []Card
```

Packages register cards separately from tools. A package might have both:
- `tools.Register(...)` - for agent
- `app.RegisterCard(...)` - for home

---

## Tools Mapping

### Markets (apps/markets.go)

```go
tools.Register(tools.Tool{
    Name:        "markets.get_price",
    Description: "Get current price for a crypto or commodity ticker",
    Category:    "markets",
    Input: map[string]tools.Param{
        "symbol": {Type: "string", Description: "Ticker (BTC, ETH, GOLD, OIL)", Required: true},
    },
    Handler: getPrice,
})

tools.Register(tools.Tool{
    Name:        "markets.list",
    Description: "List all available market tickers and their prices",
    Category:    "markets",
    Handler:     listPrices,
})
```

### Reminder (apps/reminder.go)

```go
tools.Register(tools.Tool{
    Name:        "reminder.today",
    Description: "Get today's daily reminder (Islamic verse/hadith)",
    Category:    "reminder",
    Handler:     getReminder,
})
```

### News (news/news.go)

```go
tools.Register(tools.Tool{
    Name:        "news.headlines",
    Description: "Get latest news headlines",
    Category:    "news",
    Handler:     getHeadlines,
})

tools.Register(tools.Tool{
    Name:        "news.search",
    Description: "Search news articles by keyword",
    Category:    "news",
    Input: map[string]tools.Param{
        "query": {Type: "string", Description: "Search query", Required: true},
    },
    Handler: searchNews,
})

tools.Register(tools.Tool{
    Name:        "news.read",
    Description: "Get full content of a news article",
    Category:    "news",
    Input: map[string]tools.Param{
        "url": {Type: "string", Description: "Article URL", Required: true},
    },
    Handler: readArticle,
})
```

### Video (video/video.go)

```go
tools.Register(tools.Tool{
    Name:        "video.latest",
    Description: "Get latest videos from subscribed channels",
    Category:    "video",
    Handler:     getLatest,
})

tools.Register(tools.Tool{
    Name:        "video.search",
    Description: "Search YouTube for videos",
    Category:    "video",
    Input: map[string]tools.Param{
        "query": {Type: "string", Description: "Search query", Required: true},
    },
    Handler: searchVideos,
})

tools.Register(tools.Tool{
    Name:        "video.play",
    Description: "Get URL to play a specific video",
    Category:    "video",
    Input: map[string]tools.Param{
        "video_id": {Type: "string", Description: "YouTube video ID", Required: true},
    },
    Handler: playVideo,
})
```

### Notes (notes/notes.go)

```go
tools.Register(tools.Tool{
    Name:        "notes.create",
    Description: "Create a new note",
    Category:    "notes",
    Input: map[string]tools.Param{
        "content": {Type: "string", Description: "Note content", Required: true},
        "title":   {Type: "string", Description: "Note title", Required: false},
        "tags":    {Type: "string", Description: "Comma-separated tags", Required: false},
    },
    Handler: createNote,
})

tools.Register(tools.Tool{
    Name:        "notes.list",
    Description: "List user's notes",
    Category:    "notes",
    Input: map[string]tools.Param{
        "tag":   {Type: "string", Description: "Filter by tag", Required: false},
        "limit": {Type: "number", Description: "Max results", Required: false},
    },
    Handler: listNotes,
})

tools.Register(tools.Tool{
    Name:        "notes.search",
    Description: "Search notes by keyword",
    Category:    "notes",
    Input: map[string]tools.Param{
        "query": {Type: "string", Description: "Search query", Required: true},
    },
    Handler: searchNotes,
})

tools.Register(tools.Tool{
    Name:        "notes.get",
    Description: "Get a specific note by ID",
    Category:    "notes",
    Input: map[string]tools.Param{
        "id": {Type: "string", Description: "Note ID", Required: true},
    },
    Handler: getNote,
})
```

### Mail (mail/mail.go)

```go
tools.Register(tools.Tool{
    Name:        "mail.send",
    Description: "Send an email or internal message",
    Category:    "mail",
    Input: map[string]tools.Param{
        "to":      {Type: "string", Description: "Recipient (username or email)", Required: true},
        "subject": {Type: "string", Description: "Subject line", Required: true},
        "body":    {Type: "string", Description: "Message body", Required: true},
    },
    Handler: sendMail,
})

tools.Register(tools.Tool{
    Name:        "mail.inbox",
    Description: "Check inbox for new messages",
    Category:    "mail",
    Handler:     checkInbox,
})

tools.Register(tools.Tool{
    Name:        "mail.read",
    Description: "Read a specific message",
    Category:    "mail",
    Input: map[string]tools.Param{
        "id": {Type: "string", Description: "Message ID", Required: true},
    },
    Handler: readMessage,
})
```

### Apps (apps/apps.go)

```go
tools.Register(tools.Tool{
    Name:        "apps.create",
    Description: "Create a new micro app from a description",
    Category:    "apps",
    Input: map[string]tools.Param{
        "name":        {Type: "string", Description: "App name", Required: true},
        "description": {Type: "string", Description: "What the app should do", Required: true},
    },
    Handler: createApp,
})

tools.Register(tools.Tool{
    Name:        "apps.list",
    Description: "List user's apps",
    Category:    "apps",
    Input: map[string]tools.Param{
        "query": {Type: "string", Description: "Search filter", Required: false},
    },
    Handler: listApps,
})

tools.Register(tools.Tool{
    Name:        "apps.modify",
    Description: "Modify an existing app",
    Category:    "apps",
    Input: map[string]tools.Param{
        "id":          {Type: "string", Description: "App ID", Required: true},
        "instruction": {Type: "string", Description: "Modification instructions", Required: true},
    },
    Handler: modifyApp,
})
```

### Blog (blog/blog.go)

```go
tools.Register(tools.Tool{
    Name:        "blog.latest",
    Description: "Get latest blog posts",
    Category:    "blog",
    Handler:     getLatestPosts,
})

tools.Register(tools.Tool{
    Name:        "blog.create",
    Description: "Create a new blog post",
    Category:    "blog",
    Input: map[string]tools.Param{
        "content": {Type: "string", Description: "Post content (markdown)", Required: true},
        "title":   {Type: "string", Description: "Post title", Required: false},
    },
    Handler: createPost,
})
```

### Chat (chat/chat.go)

```go
tools.Register(tools.Tool{
    Name:        "chat.ask",
    Description: "Ask a question to the AI chat",
    Category:    "chat",
    Input: map[string]tools.Param{
        "question": {Type: "string", Description: "Question to ask", Required: true},
        "topic":    {Type: "string", Description: "Topic/room", Required: false},
    },
    Handler: askChat,
})
```

### Wallet (wallet/wallet.go)

```go
tools.Register(tools.Tool{
    Name:        "wallet.balance",
    Description: "Get current wallet balance",
    Category:    "wallet",
    Handler:     getBalance,
})

tools.Register(tools.Tool{
    Name:        "wallet.deposit_address",
    Description: "Get crypto deposit address",
    Category:    "wallet",
    Input: map[string]tools.Param{
        "chain": {Type: "string", Description: "Chain (ethereum, base, arbitrum)", Required: false},
    },
    Handler: getDepositAddress,
})
```

---

## Summary: All Tools

| Category | Tool | Description |
|----------|------|-------------|
| markets | markets.get_price | Get ticker price |
| markets | markets.list | List all prices |
| reminder | reminder.today | Daily reminder |
| news | news.headlines | Latest headlines |
| news | news.search | Search articles |
| news | news.read | Read article |
| video | video.latest | Latest videos |
| video | video.search | Search YouTube |
| video | video.play | Play video |
| notes | notes.create | Create note |
| notes | notes.list | List notes |
| notes | notes.search | Search notes |
| notes | notes.get | Get note |
| mail | mail.send | Send message |
| mail | mail.inbox | Check inbox |
| mail | mail.read | Read message |
| apps | apps.create | Create app |
| apps | apps.list | List apps |
| apps | apps.modify | Modify app |
| blog | blog.latest | Latest posts |
| blog | blog.create | Create post |
| chat | chat.ask | Ask AI |
| wallet | wallet.balance | Get balance |
| wallet | wallet.deposit_address | Get deposit address |

**Total: 24 tools across 9 categories**

---

## User-Generated App Tools

Apps can register tools at runtime via JavaScript SDK:

```javascript
// In generated app code
mu.register({
    name: "expense_tracker.add",
    description: "Add an expense entry",
    input: {
        amount: { type: "number", description: "Amount spent", required: true },
        category: { type: "string", description: "Expense category", required: false }
    },
    handler: async (params) => {
        return addExpense(params.amount, params.category);
    }
});
```

When app loads, the runtime calls back to register the tool:
- Tool name prefixed with app ID or user namespace
- Handler executes in app's iframe sandbox
- Agent can discover and invoke user-created tools

This allows:
- Agent learns new capabilities as users build apps
- "Make me an expense tracker" -> app created -> `expense_tracker.add` tool available
- Agent can then use that tool: "Add $50 expense for groceries"

---

## User Context & Auth

Tools always run with user context:

```go
// Agent creates context with user
ctx := tools.WithUser(context.Background(), userID)

// Tool handler extracts user
func createNote(ctx context.Context, params map[string]any) (any, error) {
    userID := tools.UserFromContext(ctx)
    // Uses user's auth, credits, permissions
    return notes.CreateNote(userID, ...)
}
```

- Tools inherit user's credentials
- Credit costs same as direct use
- No elevated permissions for tools
- Background system tasks use system context (no user)

---

## LLM Provider Selection

**Agent uses Anthropic** - speed is critical for tool orchestration

**Chat uses Fanar** - ethical Islamic discussions, content moderation

```go
// agent/agent.go
prompt := ai.Prompt{
    Provider: ai.ProviderAnthropic,  // Fast tool execution
    // ...
}

// chat/chat.go  
prompt := ai.Prompt{
    Provider: ai.ProviderFanar,  // Ethical discussions
    // ...
}
```

---

## Future: App Builder Tool Lookup

For multi-turn app building (beyond one-shot), the builder may need to:
- Look up existing tools for reuse
- Reference SDK CSS/JS
- Read existing code for diffs

Potential tools for the builder itself:

```go
tools.Register(tools.Tool{
    Name:        "sdk.css",
    Description: "Get available CSS classes from the Mu SDK",
    Handler:     getSDKCSS,
})

tools.Register(tools.Tool{
    Name:        "sdk.js",
    Description: "Get available JavaScript APIs from the Mu SDK",
    Handler:     getSDKJS,
})

tools.Register(tools.Tool{
    Name:        "tools.list",
    Description: "List available tools the app can use",
    Handler:     listTools,
})
```

This is advanced - parking for later.

---

## Implementation Order

1. Create `tools/tools.go` with core types and registry
2. Migrate `markets` as first tool (simple)
3. Update `agent/` to use `tools.List()` and `tools.Call()`
4. Update agent to use Anthropic provider
5. Migrate remaining packages
6. Remove old `agent/tools.go` hardcoded definitions
7. Add `mu.register()` to app SDK for user-generated tools
