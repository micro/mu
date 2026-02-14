# Agent & Services Quick Reference

> **TL;DR**: This is a proposed plan (not yet implemented) for an agent system that works across microservices, enabling a marketplace of developer-created skills.

---

## The Vision in 3 Sentences

1. **Developers** create microservices using go-micro and publish them to Mu's service registry
2. **The Agent** automatically discovers these services and uses them as tools to answer user requests  
3. **Users** get an ever-growing set of capabilities, and **developers** get paid for usage

---

## Two Main Components

### 1. Agent with Dynamic Tools Registry

**What it is**: An AI agent (using Claude) that can discover and call services

**How it works**:
```
User: "What's the weather in London?"
  ↓
Agent checks tools registry
  ↓
Finds "weather.GetForecast" service
  ↓
Calls service via RPC
  ↓
Returns formatted answer to user
```

**Key innovation**: Tools registry is automatically built from service registry metadata (not hardcoded)

### 2. Service Marketplace

**What it is**: Platform where developers publish go-micro services

**Publishing flow**:
1. Developer creates service using go-micro
2. Defines metadata (methods, inputs, outputs, pricing)
3. Deploys service to their infrastructure
4. Registers with Mu (POST to `/api/services/register`)
5. Service is immediately available to agents

**Revenue model**: 70% developer, 30% platform (per call)

---

## Example Use Case

**Weather Service**

```json
{
  "service": {
    "name": "Weather Service",
    "endpoint": "grpc://weather.example.com:50051",
    "cost": 2
  },
  "methods": [{
    "name": "GetForecast",
    "description": "Get weather forecast for a location",
    "input": {
      "location": {"type": "string", "required": true},
      "days": {"type": "number", "default": 5}
    }
  }]
}
```

When registered:
- Agent can discover it
- Users can ask weather questions
- Developer earns 1.4p per call (70% of 2 credits)
- Platform earns 0.6p per call (30% of 2 credits)

---

## Standardized Protocol

All services MUST:
- ✅ Implement health check endpoint
- ✅ Follow standard request/response format
- ✅ Provide metadata with method schemas
- ✅ Handle errors gracefully

**Recommended**: gRPC (efficient, typed)  
**Alternative**: HTTP JSON-RPC (simpler)

---

## Implementation Roadmap

| Phase | Duration | Deliverables |
|-------|----------|--------------|
| **1. Foundation** | 2-3 weeks | Service registry, basic agent, 1 example service |
| **2. Tools Registry** | 1-2 weeks | Dynamic tool discovery, health monitoring |
| **3. Protocol & SDK** | 2-3 weeks | Service template, SDK, 5 example services |
| **4. Marketplace** | 2-3 weeks | UI, search, ratings, revenue tracking |
| **5. Polish** | Ongoing | Monitoring, security, scale |

**Total**: 3-4 months to MVP

---

## Key Benefits

**For Users**:
- 🚀 Agent gets more capable over time
- 💰 Pay only for what you use
- 🔒 Privacy-respecting (no ads, no tracking)

**For Developers**:
- 💵 Earn money from your services
- 🛠️ Easy to build (standard template + SDK)
- 📈 Built-in distribution (marketplace)

**For Platform**:
- 🔄 Network effects (more services = more users)
- 💸 Sustainable revenue (30% of transactions)
- 🌟 Differentiation (unique marketplace model)

---

## Open Questions to Resolve

1. **Technical**:
   - Service versioning strategy?
   - Pull vs push for service discovery?
   - How does agent choose between similar services?

2. **Business**:
   - Is 70/30 revenue split fair?
   - Who approves new services (auto vs manual)?
   - Can services call other services?

3. **Product**:
   - How transparent should agent be about tool use?
   - How much autonomy should agent have?
   - Support for long-running tasks?

---

## Success Criteria

The system succeeds if:

✅ **Agent capability** grows over time as services are added  
✅ **Developer experience** makes it easy and profitable to create services  
✅ **Platform sustainability** through network effects and revenue sharing

---

## Next Steps

1. **Review** this plan with stakeholders
2. **Validate** technical approach with a prototype
3. **Define** Phase 1 milestones in detail
4. **Begin** implementation

---

## Full Documentation

For complete architecture details, see:
- [Full Architecture Plan](AGENT_SERVICES_ARCHITECTURE.md) - Comprehensive design document
- [System Design](SYSTEM_DESIGN.md) - Current Mu architecture
- [API Reference](API_COVERAGE.md) - Existing API endpoints

---

*Quick Reference created: February 2026*  
*Status: Proposal / Planning*
