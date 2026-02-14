# Agent and Services Architecture Plan

> **Status**: Design Document - Not Yet Implemented
>
> This document outlines a proposed architecture for an agent system that can work across microservices, enabling a marketplace of skills and capabilities.

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Problem Statement](#problem-statement)
3. [Architecture Overview](#architecture-overview)
4. [Component 1: Agent with Tools Registry](#component-1-agent-with-tools-registry)
5. [Component 2: Service Publishing & Discovery](#component-2-service-publishing--discovery)
6. [Service Protocol Specification](#service-protocol-specification)
7. [Marketplace Model](#marketplace-model)
8. [Integration with Mu Platform](#integration-with-mu-platform)
9. [Implementation Roadmap](#implementation-roadmap)
10. [Security Considerations](#security-considerations)
11. [Open Questions](#open-questions)

---

## Executive Summary

This plan proposes a two-component architecture for Mu:

1. **Agent System**: An AI agent that can discover and use services as tools, with a tools registry dynamically built from a service registry
2. **Service Marketplace**: A platform where developers can publish go-micro services that agents can consume, with monetization capabilities

The goal is to create an extensible ecosystem where:
- Mu serves as the front-end interface
- Developers create services using the go-micro framework
- Agents can dynamically discover and use these services
- Service creators can profit from usage
- Users benefit from an ever-growing marketplace of capabilities

---

## Problem Statement

### Historical Context

Mu previously had an agent implementation but it wasn't aligned with needs. The previous approach was:
- Too monolithic
- Not extensible enough
- Didn't leverage the microservices architecture

### New Vision

We need an agent that can:
- Work across a distributed set of services
- Dynamically discover available capabilities
- Expose microservices as tools/skills
- Enable a marketplace where people can:
  - Publish services
  - Share services
  - Profit from usage

### Key Questions to Answer

1. Is there a feasible model where Mu is the frontend and services are created via go-micro?
2. How do we know the location of services?
3. What is the standardized protocol for interaction?
4. How do we build a tools registry from a service registry?
5. How do we enable monetization and profit-sharing?

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                         Mu Frontend                          │
│                     (User Interface)                         │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                      Agent Layer                             │
│  ┌────────────────┐         ┌──────────────────┐           │
│  │  Agent Core    │◄────────┤  Tools Registry  │           │
│  │  (Claude/LLM)  │         │  (Dynamic)       │           │
│  └────────────────┘         └──────────────────┘           │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                   Service Registry                           │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐            │
│  │  Service   │  │  Service   │  │  Service   │            │
│  │  Metadata  │  │  Metadata  │  │  Metadata  │  ...       │
│  └────────────┘  └────────────┘  └────────────┘            │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                  Go-Micro Services                           │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐            │
│  │  Weather   │  │  Translate │  │  Analytics │            │
│  │  Service   │  │  Service   │  │  Service   │  ...       │
│  └────────────┘  └────────────┘  └────────────┘            │
└─────────────────────────────────────────────────────────────┘
```

### Data Flow

1. **User Request** → Mu Frontend
2. **Frontend** → Agent Core (natural language request)
3. **Agent Core** queries **Tools Registry** for available capabilities
4. **Tools Registry** is built from **Service Registry** metadata
5. **Agent Core** selects appropriate service(s) to call
6. **Agent Core** → **Go-Micro Service** (standardized RPC)
7. **Service** processes request and returns result
8. **Agent Core** → **Mu Frontend** (formatted response)

---

## Component 1: Agent with Tools Registry

### Agent Core

The agent is the orchestration layer that:
- Receives user requests in natural language
- Understands intent
- Discovers available tools/services
- Plans and executes multi-step workflows
- Returns results to users

**Technology Stack:**
- LLM: Anthropic Claude (already integrated in Mu)
- Tool Use: Claude's native tool/function calling API
- Context: Conversation history + RAG if needed

### Tools Registry (Dynamic)

The tools registry is NOT static. It's dynamically generated from the service registry.

**Structure:**

```go
type Tool struct {
    Name        string                 // e.g., "weather.get_forecast"
    Description string                 // What the tool does
    Parameters  map[string]Parameter   // Input schema
    ServiceID   string                 // Link back to service
    Endpoint    string                 // Service RPC endpoint
    Cost        int                    // Credits per call
}

type Parameter struct {
    Type        string   // "string", "number", "boolean", "object", "array"
    Description string   // What this parameter is for
    Required    bool     // Is it required?
    Enum        []string // Optional: allowed values
}
```

**Registry Operations:**

```go
// ToolsRegistry interface
type ToolsRegistry interface {
    // List all available tools
    List() ([]Tool, error)
    
    // Get a specific tool by name
    Get(name string) (*Tool, error)
    
    // Refresh from service registry
    Refresh() error
    
    // Convert to Claude tool format
    ToClaudeTools() ([]ClaudeTool, error)
}
```

**Tool Discovery Flow:**

1. Service registers with service registry (includes metadata)
2. Tools registry polls/subscribes to service registry
3. For each service, extract tool definitions from metadata
4. Build tools in Claude-compatible format
5. Agent queries tools registry when planning actions
6. Tools registry returns available tools with descriptions

### Agent Workflow

```
┌──────────────────────┐
│   User Request       │
│  "What's the weather │
│   in London?"        │
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│  Agent: Parse Intent │
│  - Need weather data │
│  - Location: London  │
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│  Query Tools Registry│
│  - Find weather tools│
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│  Select Tool         │
│  weather.get_forecast│
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│  Call Service        │
│  via RPC             │
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│  Format Response     │
│  Return to User      │
└──────────────────────┘
```

---

## Component 2: Service Publishing & Discovery

### Service Registry

The service registry is the source of truth for all available services.

**Structure:**

```go
type Service struct {
    ID          string            // Unique service identifier
    Name        string            // Human-readable name
    Version     string            // Semantic version
    Description string            // What it does
    Author      string            // Creator user ID
    
    // Location
    Endpoint    string            // RPC endpoint URL
    Protocol    string            // "grpc", "http", etc.
    
    // Metadata for tool generation
    Methods     []ServiceMethod   // Available RPC methods
    
    // Marketplace
    Cost        int               // Credits per call
    Revenue     RevenueShare      // How revenue is split
    
    // Quality & Trust
    Rating      float64           // Average user rating
    Calls       int64             // Total calls made
    Uptime      float64           // Service uptime %
    
    // Lifecycle
    Status      string            // "active", "deprecated", "offline"
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type ServiceMethod struct {
    Name        string                // e.g., "GetForecast"
    Description string                // What it does
    Input       map[string]Parameter  // Input schema
    Output      map[string]Parameter  // Output schema
    Examples    []Example             // Usage examples
}

type Example struct {
    Input       string  // JSON input
    Output      string  // JSON output
    Description string  // What this example shows
}

type RevenueShare struct {
    Author      int  // Percentage to service author
    Platform    int  // Percentage to Mu platform
}
```

**Registry Operations:**

```go
// ServiceRegistry interface
type ServiceRegistry interface {
    // Publishing
    Register(service *Service) error
    Update(serviceID string, service *Service) error
    Unregister(serviceID string) error
    
    // Discovery
    List(filters ...Filter) ([]Service, error)
    Get(serviceID string) (*Service, error)
    Search(query string) ([]Service, error)
    
    // Health
    HealthCheck(serviceID string) error
    UpdateStatus(serviceID string, status string) error
    
    // Analytics
    RecordCall(serviceID string) error
    GetStats(serviceID string) (*Stats, error)
}
```

### Service Publishing Flow

```
┌──────────────────────────────────────┐
│  1. Developer Creates Service        │
│     - Uses go-micro framework        │
│     - Implements standard interface  │
│     - Defines methods & schemas      │
└──────────────┬───────────────────────┘
               │
               ▼
┌──────────────────────────────────────┐
│  2. Service Metadata Definition      │
│     - service.json or .proto         │
│     - Methods, inputs, outputs       │
│     - Pricing, description           │
└──────────────┬───────────────────────┘
               │
               ▼
┌──────────────────────────────────────┐
│  3. Deploy Service                   │
│     - Run on cloud/own infrastructure│
│     - Get public endpoint URL        │
└──────────────┬───────────────────────┘
               │
               ▼
┌──────────────────────────────────────┐
│  4. Register with Mu                 │
│     - POST /api/services/register    │
│     - Provide metadata & endpoint    │
│     - Service validated & listed     │
└──────────────┬───────────────────────┘
               │
               ▼
┌──────────────────────────────────────┐
│  5. Available to Agents              │
│     - Listed in service registry     │
│     - Tools generated automatically  │
│     - Agent can discover & use       │
└──────────────────────────────────────┘
```

### Service Discovery Flow

**Pull-based (Polling):**
```
Tools Registry ────(every 60s)───► Service Registry
               ◄────(services)────
```

**Push-based (WebSocket/Events):**
```
Service Registry ──(on change)──► Tools Registry
                               (immediate update)
```

Recommendation: **Hybrid approach**
- Initial poll on startup
- Subscribe to updates for real-time changes
- Periodic health checks (every 5 minutes)

---

## Service Protocol Specification

### Standard Service Interface

All services MUST implement a standard interface to be consumable by agents.

**Protocol Options:**

1. **gRPC** (Recommended)
   - Strong typing via protobuf
   - Efficient binary protocol
   - Built-in streaming
   - Language agnostic
   
2. **HTTP/JSON-RPC**
   - Simpler to implement
   - Easy debugging
   - Works everywhere
   - Less efficient

**Recommendation**: Start with gRPC for production, support JSON-RPC for easier onboarding.

### Service Metadata Format

Services provide metadata via a standardized descriptor:

```json
{
  "service": {
    "id": "weather-v1",
    "name": "Weather Service",
    "version": "1.0.0",
    "description": "Get weather forecasts and current conditions",
    "author": "user123",
    "endpoint": "grpc://weather.services.mu.xyz:50051",
    "protocol": "grpc",
    "cost": 2
  },
  "methods": [
    {
      "name": "GetForecast",
      "description": "Get weather forecast for a location",
      "input": {
        "location": {
          "type": "string",
          "description": "City name or coordinates",
          "required": true
        },
        "days": {
          "type": "number",
          "description": "Number of days to forecast (1-10)",
          "required": false,
          "default": 5
        }
      },
      "output": {
        "forecast": {
          "type": "array",
          "description": "Array of daily forecasts",
          "items": {
            "type": "object",
            "properties": {
              "date": {"type": "string"},
              "temp_high": {"type": "number"},
              "temp_low": {"type": "number"},
              "conditions": {"type": "string"}
            }
          }
        }
      },
      "examples": [
        {
          "input": "{\"location\": \"London\", \"days\": 3}",
          "output": "{\"forecast\": [{\"date\": \"2026-02-04\", \"temp_high\": 12, \"temp_low\": 5, \"conditions\": \"Cloudy\"}]}",
          "description": "3-day forecast for London"
        }
      ]
    }
  ]
}
```

### Service Communication Protocol

**Request Format:**

```json
{
  "service": "weather-v1",
  "method": "GetForecast",
  "parameters": {
    "location": "London",
    "days": 3
  },
  "context": {
    "user_id": "user123",
    "session_id": "sess_abc",
    "request_id": "req_xyz"
  }
}
```

**Response Format:**

```json
{
  "success": true,
  "data": {
    "forecast": [...]
  },
  "metadata": {
    "latency_ms": 234,
    "credits_used": 2
  }
}
```

**Error Format:**

```json
{
  "success": false,
  "error": {
    "code": "INVALID_LOCATION",
    "message": "Location 'Xyz' not found",
    "retryable": false
  }
}
```

### Health Check Protocol

All services MUST implement a health check endpoint:

```
GET /health
```

Response:
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime_seconds": 86400,
  "last_error": null
}
```

---

## Marketplace Model

### Revenue Sharing

**Default Split:**
- Service Author: 70%
- Mu Platform: 30%

Authors can set their own pricing (in credits):
- Minimum: 1 credit (1p)
- No maximum

### Service Tiers

**Free Tier:**
- 10 calls/day per user
- Good for trying services

**Paid:**
- Pay-per-call with credits
- Bulk discounts possible

**Subscription** (future):
- Monthly fee for unlimited calls to specific service
- Good for power users

### Discoverability

Services are discoverable via:

1. **Agent Discovery**: Automatic, based on user intent
2. **Service Marketplace UI**: Browse all services
3. **Search**: Find services by keyword, category
4. **Recommendations**: "Users who used X also used Y"
5. **Featured**: Curated list of high-quality services

### Quality Metrics

Users rate services on:
- **Accuracy**: Did it work correctly?
- **Speed**: Was it fast enough?
- **Value**: Worth the credits?

Services with low ratings may be:
- Demoted in search results
- Flagged for review
- Removed if consistently poor

### Example Marketplace Categories

- **Data & Information**: Weather, stocks, news
- **Translation & Language**: Translate text, detect language
- **Image Processing**: Resize, filter, OCR
- **Text Processing**: Summarize, analyze sentiment
- **Communication**: Send SMS, email, push notifications
- **Analytics**: Parse logs, generate reports
- **Utilities**: Convert formats, validate data

---

## Integration with Mu Platform

### Authentication & Authorization

**Service Access:**
- Services authenticated via API keys
- Keys linked to user accounts
- Credits deducted from user wallet

**User Context:**
- User ID passed to services
- Services can track per-user data
- Privacy: Services see user ID, not personal info

### Credit System Integration

```go
// Before service call
func (a *Agent) CallService(serviceID string, params map[string]interface{}) (interface{}, error) {
    // 1. Get service metadata
    service, err := a.registry.Get(serviceID)
    if err != nil {
        return nil, err
    }
    
    // 2. Check user has enough credits
    if user.Credits < service.Cost {
        return nil, ErrInsufficientCredits
    }
    
    // 3. Call service
    result, err := a.callServiceRPC(service, params)
    if err != nil {
        return nil, err
    }
    
    // 4. Deduct credits
    wallet.DeductCredits(user.ID, service.Cost)
    
    // 5. Credit service author
    wallet.CreditUser(service.Author, service.Cost * 0.7)
    
    // 6. Record call for analytics
    registry.RecordCall(serviceID)
    
    return result, nil
}
```

### UI Integration

**Agent Chat Interface:**
```
User: "What's the weather in London?"

Agent: [queries tools registry]
       [finds weather.GetForecast]
       [calls service]
       
Agent: "It's currently 12°C and cloudy in London. 
        The forecast for the next 3 days is..."
        
        [Used weather.GetForecast - 2 credits]
```

**Service Marketplace Page:**
```
/services
  - Browse all services
  - Search by keyword
  - Filter by category, price, rating
  - View service details
  - Try service (with examples)
```

**Developer Dashboard:**
```
/services/my-services
  - List my published services
  - View analytics (calls, revenue, ratings)
  - Update service metadata
  - Monitor health status
```

### Data Storage

**Service Registry:**
```
~/.mu/data/services.db (SQLite)
  - services table
  - service_methods table
  - service_calls table (analytics)
  - service_ratings table
```

**Tools Registry:**
- In-memory cache
- Rebuilt from service registry on startup
- Updated on service changes

---

## Implementation Roadmap

### Phase 1: Foundation (2-3 weeks)

**Goal**: Basic service registry and agent

- [ ] Design service metadata schema
- [ ] Implement service registry (SQLite)
- [ ] Create service registration API
- [ ] Build basic agent with tool selection
- [ ] Integrate agent with Claude's tool API
- [ ] Create one example service (weather)

**Deliverables**:
- Service registry database
- `/api/services` endpoints
- Agent that can call one service
- Documentation for service developers

### Phase 2: Tools Registry (1-2 weeks)

**Goal**: Dynamic tool discovery

- [ ] Implement tools registry
- [ ] Auto-generate Claude tools from service metadata
- [ ] Service health monitoring
- [ ] Tools registry refresh mechanism
- [ ] Error handling & retries

**Deliverables**:
- Tools registry that polls service registry
- Agent can discover and use multiple services
- Health check system

### Phase 3: Protocol & SDK (2-3 weeks)

**Goal**: Make it easy to create services

- [ ] Define standard gRPC protocol
- [ ] Create go-micro service template
- [ ] Build SDK for common tasks
- [ ] Write developer documentation
- [ ] Create 3-5 example services

**Deliverables**:
- Service SDK (Go package)
- Service template generator
- Developer guide
- Example services (translate, image, analytics)

### Phase 4: Marketplace (2-3 weeks)

**Goal**: Users can discover and use services

- [ ] Build service marketplace UI
- [ ] Service search and filtering
- [ ] Service detail pages
- [ ] User ratings and reviews
- [ ] Revenue tracking and payouts

**Deliverables**:
- `/services` marketplace page
- Service detail pages
- Rating system
- Developer dashboard

### Phase 5: Polish & Scale (Ongoing)

**Goal**: Production-ready

- [ ] Performance optimization
- [ ] Monitoring and alerting
- [ ] Service approval/moderation
- [ ] Enhanced security
- [ ] Load balancing for popular services
- [ ] Caching layer
- [ ] Rate limiting

**Deliverables**:
- Production monitoring
- Security audit
- Performance benchmarks
- Scalability plan

---

## Security Considerations

### Service Validation

**Before Registration:**
- Validate service metadata schema
- Check endpoint is reachable
- Verify author identity
- Scan for malicious code (if open source)

**Ongoing:**
- Monitor for suspicious activity
- Rate limit per service
- Circuit breakers for failing services
- Quarantine services with poor ratings

### Data Privacy

**User Data:**
- Services receive user ID only, not personal info
- No PII shared unless user explicitly provides
- Services cannot access other user data

**Service Data:**
- Services may store request history
- Must comply with privacy policy
- Users can request data deletion

### API Security

**Service Authentication:**
- All service calls require valid API key
- Keys scoped to user account
- Keys can be revoked

**Rate Limiting:**
- Per-user limits (prevent abuse)
- Per-service limits (protect services)
- Global limits (protect platform)

### Credit Fraud Prevention

**Double-Spending:**
- Atomic credit deduction
- Transaction logging
- Rollback on service failure

**Service Fraud:**
- Services must prove they did work
- Random audits of service responses
- Dispute resolution process

---

## Open Questions

### Technical

1. **Service Versioning**: How do we handle breaking changes?
   - Semantic versioning?
   - Support multiple versions simultaneously?
   
2. **Service Discovery**: Pull vs Push?
   - Polling (simple, eventually consistent)
   - WebSockets (real-time, more complex)
   - Hybrid?

3. **Protocol**: gRPC vs HTTP?
   - gRPC: Efficient, typed, streaming
   - HTTP: Simple, universal, easy debugging
   - Support both?

4. **Tool Selection**: How does agent choose between similar tools?
   - Cost optimization?
   - Quality (ratings)?
   - Speed?
   - User preference?

### Business

1. **Revenue Split**: Is 70/30 fair?
   - Industry standard?
   - Adjustable per service?
   - Volume discounts?

2. **Quality Control**: Who approves new services?
   - Automatic approval?
   - Manual review?
   - Community moderation?

3. **Service Dependencies**: Can services call other services?
   - Nested revenue sharing?
   - Infinite loops?
   - Cost estimation?

4. **Service Hosting**: Where do services run?
   - Developer infrastructure (flexible, complex)
   - Mu infrastructure (simple, limited)
   - Hybrid?

### Product

1. **User Experience**: How do users know what's happening?
   - Show tool calls?
   - Transparent pricing?
   - Undo/refund?

2. **Agent Behavior**: How much autonomy?
   - Auto-use services?
   - Always ask permission?
   - Configurable?

3. **Service Types**: What kinds of services make sense?
   - Read-only (safe)?
   - Write operations (risky)?
   - Long-running tasks?
   - Webhooks/callbacks?

---

## Conclusion

This architecture provides a path forward for building an agent system that:

✅ Works across distributed microservices
✅ Dynamically discovers available capabilities
✅ Enables a marketplace of skills
✅ Allows developers to profit from their services
✅ Integrates cleanly with existing Mu platform

### Success Criteria

The system is successful if:

1. **For Users**: Agent becomes more capable over time as services are added
2. **For Developers**: Easy to create and profitable to run services
3. **For Platform**: Creates network effects and sustainable revenue

### Next Steps

1. Review this plan with stakeholders
2. Validate technical approach with prototype
3. Define Phase 1 milestones in detail
4. Begin implementation

### Estimated Timeline

- **Phase 1-2**: 1-2 months (core foundation)
- **Phase 3-4**: 1-2 months (SDK and marketplace)
- **Phase 5**: Ongoing (polish and scale)

**Total**: 3-4 months to MVP, 6+ months to production-ready marketplace

---

*Last Updated: February 2026*
*Status: Proposal / Design Document*
