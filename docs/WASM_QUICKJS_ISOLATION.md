# WebAssembly and QuickJS Isolation for Micro Apps

## Overview

This document explores using WebAssembly (WASM) and QuickJS as an alternative to iframe-based isolation for running user-generated micro apps in Mu.

## Current Implementation

Currently, Mu apps run in sandboxed iframes:
```html
<iframe sandbox="allow-scripts allow-same-origin allow-forms" src="/apps/{id}/preview"></iframe>
```

**Advantages:**
- Native browser security boundary
- Well-tested isolation model
- Good performance for DOM manipulation
- Direct access to browser APIs
- Zero additional dependencies

**Disadvantages:**
- Same-origin policy complications
- Larger memory footprint per app
- Limited control over execution environment
- Potential for XSS if sandbox is misconfigured

## WASM + QuickJS Alternative

### Architecture

QuickJS is a small, embeddable JavaScript engine that can be compiled to WebAssembly. This provides:

1. **Isolated JavaScript execution**: Each app runs in its own QuickJS VM
2. **Controlled API surface**: Only expose explicitly defined APIs
3. **Memory limits**: Can enforce per-app memory constraints
4. **CPU limits**: Can terminate long-running scripts
5. **No DOM access**: Apps interact via message passing

### Implementation Approach

```
┌─────────────────────────────────────────┐
│         Browser (Main Thread)           │
│  ┌───────────────────────────────────┐  │
│  │   Mu Host Application (Go)        │  │
│  │  ┌─────────────────────────────┐  │  │
│  │  │ QuickJS WASM Runtime        │  │  │
│  │  │  ┌───────────────────────┐  │  │  │
│  │  │  │  User App Code (JS)   │  │  │  │
│  │  │  └───────────────────────┘  │  │  │
│  │  └─────────────────────────────┘  │  │
│  └───────────────────────────────────┘  │
└─────────────────────────────────────────┘
```

### Performance Considerations

#### Speed Comparison

**QuickJS WASM:**
- ✅ Smaller memory footprint per instance (~100KB-500KB)
- ✅ Faster startup time for pure JS apps
- ✅ Better CPU isolation and control
- ❌ Slower DOM manipulation (requires message passing)
- ❌ No direct browser API access
- ❌ Additional overhead for WASM<->JS boundary crossing

**Iframe:**
- ✅ Direct DOM access (fast rendering)
- ✅ Full browser API access
- ✅ Optimized by browser engines
- ❌ Higher memory per instance (~5MB+)
- ❌ Slower iframe initialization

**Verdict**: 
- For **DOM-heavy apps** (most Mu apps): iframes are faster
- For **computation-heavy apps** with minimal UI: QuickJS WASM could be faster
- For **security-critical isolation**: QuickJS WASM provides stronger guarantees

### Security Comparison

| Feature | Iframe Sandbox | QuickJS WASM |
|---------|----------------|--------------|
| DOM isolation | ✓ | ✓ |
| Network isolation | Partial (sandbox) | ✓ (complete control) |
| Memory isolation | ✓ (process-level) | ✓ (VM-level) |
| CPU limits | ❌ | ✓ (can set timeouts) |
| File system access | ❌ (sandboxed) | ❌ (no access) |
| Side-channel attacks | Vulnerable | More resistant |
| Escape vulnerabilities | Browser-dependent | QuickJS-dependent |

### Use Cases

**Best for Iframes:**
- Rich UI applications
- Apps needing direct DOM manipulation
- Apps using browser APIs (Canvas, WebGL, etc.)
- Quick development iteration

**Best for QuickJS WASM:**
- Computation-heavy algorithms
- Data processing apps
- Apps requiring strict resource limits
- Multi-tenant environments with many concurrent apps

## Recommendation for Mu

**Keep iframe-based execution** because:

1. **User apps are UI-focused**: Todo lists, timers, expense trackers - all heavily manipulate DOM
2. **Development velocity**: Users expect to write standard web code with direct DOM access
3. **Browser API access**: Apps benefit from Canvas, localStorage, fetch, etc.
4. **Proven security**: Browser sandbox has years of security research and patches
5. **No additional complexity**: No WASM runtime to maintain

**When to reconsider:**

1. If running hundreds of apps simultaneously (memory becomes issue)
2. If strict CPU limits are needed (prevent infinite loops)
3. If targeting non-browser environments (server-side rendering)
4. If security requirements exceed browser sandbox capabilities

## Hybrid Approach

A middle ground could be:

1. **Default**: Keep iframe execution for most apps
2. **Optional**: Allow marking apps as "compute-only" to run in QuickJS WASM
3. **Server-side**: Use QuickJS for server-side app execution/validation

Example:
```go
type App struct {
    // ... existing fields
    Runtime string `json:"runtime"` // "iframe" (default) or "quickjs"
}
```

## Implementation Resources

If pursuing QuickJS WASM:

- **quickjs-emscripten**: https://github.com/justjake/quickjs-emscripten
- **QuickJS official**: https://bellard.org/quickjs/
- **Integration examples**: https://github.com/suchipi/quickjs-wasm

## Conclusion

For Mu's current use case (user-generated UI apps), **iframe-based execution is the superior choice**. QuickJS WASM would add complexity without providing meaningful benefits for the typical Mu app (which manipulates DOM heavily).

Focus performance improvements on:
1. Optimizing app generation (LLM calls) - see Fanar performance improvements
2. Caching compiled apps
3. Lazy loading apps on the home page
4. Prefetching app data in the background

The real performance bottleneck is **app generation via Fanar API**, not runtime execution.
