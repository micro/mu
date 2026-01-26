# Mu Flow - Automation Language

> Human-readable automations that run without AI

## Overview

Mu Flow is a simple automation language designed for LLM generation and human editing. The agent interprets your intent, generates a flow, and you can inspect, edit, and schedule it to run automatically.

**Key insight:** Natural language is ambiguous. Code is precise but hard to read. Flow syntax sits in the middle - predictable enough to parse deterministically, readable enough for humans to edit.

```
every day at 7am:
    get reminder
    then search news for "tech"
    then email to me with subject "Morning Briefing"
```

## Why Flows?

| Problem | Solution |
|---------|----------|
| LLM calls are expensive | Flows run without LLM once created |
| Natural language is ambiguous | Flow syntax is deterministic |
| Hard to debug AI behavior | Flows show exactly what will happen |
| Can't share AI conversations | Flows are exportable recipes |
| AI can't run on schedule | Flows execute via cron |

## Syntax

### Triggers

Flows can be triggered manually or on schedule:

```
# Daily at specific time
every day at 7am:
    ...

# Hourly
every hour:
    ...

# Weekly
every monday at 9am:
    ...

# Natural language times
every morning:
    ...

every evening:
    ...

# No trigger = manual only
get btc price
then email to me
```

### Steps

Each step maps to a tool in the registry:

| Flow Syntax | Tool | Description |
|-------------|------|-------------|
| `get reminder` | reminder.today | Daily Islamic reminder |
| `search news for "query"` | news.search | Search news articles |
| `get headlines` | news.headlines | Latest headlines |
| `email to me` | mail.send | Send to your email |
| `email to "addr"` | mail.send | Send to specific address |
| `get btc price` | markets.get_price | Get crypto price |
| `get price of "ETH"` | markets.get_price | Get specific price |
| `save note "text"` | notes.create | Save a note |
| `get balance` | wallet.balance | Check credit balance |
| `summarize` | (built-in) | Summarize previous result |

### Chaining

Steps are connected with `then`. Data flows from one step to the next:

```
search news for "bitcoin"
then summarize
then email to me
```

The news search results flow into summarize, which flows into the email body.

### Variables

Save step results to variables for later use:

```
get btc price save as btc
get eth price save as eth
email to me
```

Or save the previous result:

```
get btc price
save as btc
```

### Comments

Lines starting with `#` are ignored:

```
# Morning briefing flow
every day at 7am:
    get reminder
    # TODO: add weather
    then email to me
```

## Examples

### Morning Briefing
```
every day at 7am:
    get reminder
    then search news for "tech"
    then summarize
    then email to me with subject "Morning Briefing"
```

### Price Alert (Manual)
```
get btc price
then email to "alerts@example.com" with subject "BTC Price Update"
```

### Weekly Digest
```
every friday at 5pm:
    search news for "AI"
    then summarize
    then save note "Weekly AI digest"
```

## Architecture

### How It Works

```
┌─────────────────┐
│  User Request   │  "every morning email me crypto news"
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│     Agent       │  LLM interprets intent
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   Flow Source   │  every day at 8am:
│                 │      search news for "crypto"
│                 │      then email to me
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│     Parser      │  Converts to structured steps
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│    Executor     │  Calls tools from registry
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Tool Registry  │  news.search, mail.send, etc.
└─────────────────┘
```

### Components

| File | Purpose |
|------|----------|
| `flow/flow.go` | Data model, storage |
| `flow/parser.go` | Syntax → structured steps |
| `flow/executor.go` | Run flows via tools |
| `flow/tools.go` | flow.create/list/run/delete |
| `flow/handler.go` | /flows web UI |
| `flow/scheduler.go` | Background cron runner (TODO) |

### Storage

Flows are stored as JSON in `~/.mu/data/flows/`:

```json
{
  "id": "flow_1234567890",
  "user_id": "alice",
  "name": "Morning Briefing",
  "source": "every day at 7am:\n    get reminder\n    then email to me",
  "schedule": "0 7 * * *",
  "enabled": true,
  "run_count": 42,
  "last_run": "2026-01-26T07:00:00Z"
}
```

## Apps Integration

### Current State

Flows call tools from the registry. Built-in tools include:
- `news.*` - News search and headlines
- `mail.*` - Send and check email
- `markets.*` - Crypto/stock prices
- `notes.*` - Personal notes
- `reminder.*` - Islamic reminders
- `wallet.*` - Credit balance
- `apps.*` - Create/modify apps

### Future: App-Generated Tools

User-created apps can register tools:

```javascript
// In app code
mu.register({
  name: "weather.forecast",
  description: "Get weather forecast",
  params: { city: "string" },
  handler: async (params) => {
    return await mu.fetch(`/api/weather/${params.city}`);
  }
});
```

This would allow flows like:
```
every morning:
    get weather for "London"
    then email to me
```

### App Generation in Flows (Planned)

Flows could trigger app creation:

```
every day at 9am:
    create app "Daily Stats" showing my usage
    then email link to me
```

The agent generates the app code, the app runs to produce output, and the result flows to the next step.

## Roadmap

### Phase 1: Foundation ✅
- [x] Flow data model and storage
- [x] Parser for basic syntax
- [x] Executor calling tools registry
- [x] Web UI for viewing/managing flows
- [x] Agent integration (flow.create tool)

### Phase 2: Scheduling ✅
- [x] Background scheduler (cron-like)
- [x] Execution history and logs (last 10 runs)
- [x] Pre-built templates (Morning Briefing, etc.)
- [ ] Retry on failure
- [ ] Notification on completion/failure

### Phase 3: Advanced Syntax (In Progress)
- [x] Variables: `save as myvar` modifier
- [ ] Conditionals: `if price > 100 then...`
- [ ] Event triggers: `when btc crosses 100000`
- [ ] Loops: `for each article in headlines`

### Phase 4: App Integration
- [ ] Apps register custom tools
- [ ] Flow-triggered app generation
- [ ] App output as flow input
- [ ] Marketplace for flow templates

## Design Decisions

### Why Not Just Use Code?

Code is powerful but:
1. Most users can't write it
2. LLMs make syntax errors
3. Hard to audit what it does
4. Security concerns with arbitrary execution

Flows are constrained to calling registered tools - safe by design.

### Why Not Just Use Natural Language?

Natural language is:
1. Ambiguous ("send it to me" - send what?)
2. Requires LLM to interpret every time
3. Can't be scheduled without AI
4. Hard to edit precisely

Flows are the compiled output of natural language - deterministic and inspectable.

### Why Keyword-Based Syntax?

We considered:
- **Lisp-like**: `(email (search news "tech") to: me)` - powerful but alien
- **JSON**: Structured but not human-friendly
- **YAML**: Verbose and indentation-sensitive
- **Natural language**: Ambiguous

Keyword-based reads naturally: "search news for tech then email to me"

### Tool Mapping Ambiguity

The challenge: mapping natural phrases to specific tools.

"check my balance" could mean:
- `wallet.balance` (credits)
- `markets.get_price` (crypto holdings)
- A user's custom banking app

Current approach: explicit patterns in parser. Future: let user define aliases or use context from previous interactions.

## API

### Create Flow
```bash
curl -X POST https://mu.xyz/flows \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Morning Briefing",
    "source": "every day at 7am:\n    get reminder\n    then email to me"
  }'
```

### List Flows
```bash
curl https://mu.xyz/flows -H "Accept: application/json"
```

### Run Flow
```bash
curl -X POST https://mu.xyz/flows/{id}/run
```

### Delete Flow
```bash
curl -X DELETE https://mu.xyz/flows/{id}
```

## See Also

- [Micro Apps SDK](sdk) - Building apps for Mu
- [Tools Registry](/tools) - Available tools
- [Agent](/agent) - AI assistant that generates flows
