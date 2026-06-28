# gomicro-weather

A spike: mu's **weather** capability rebuilt as a [go-micro v6](https://go-micro.dev)
service, to test whether mu's services and agents can become go-micro services
and agents cleanly.

This is a **separate Go module** (its own `go.mod`). It does not add
`go-micro.dev/v6` to mu's main binary — it's a throwaway proof, not product.

## Run

```bash
cd examples/gomicro-weather
go run .                       # sections 1–3, no API key needed
ATLAS_API_KEY=sk-... go run .  # also runs section 4 (a real agent)
```

> **Behind a proxy:** go-micro's HTTP transport honours `HTTP(S)_PROXY` and
> advertises a non-loopback IP. If you have a proxy set (e.g. a sandbox), the
> in-process RPC dial gets hijacked. Set `NO_PROXY` to include loopback and the
> advertised IP, e.g. `NO_PROXY=127.0.0.1,0.0.0.0,localhost`. This is an
> operational note that matters for mu's own deployment environment.

## What it shows

1. **The conversion is a method.** mu's `weather.ForecastText(lat, lon)` plus a
   hand-registered tool in `internal/api/mcp.go` becomes one method:
   `func (Weather) Forecast(ctx, *ForecastRequest, *ForecastResponse) error`.
2. **It's a normal RPC service** — a direct typed `client.Call` works.
3. **The AI tool is auto-derived** from the method signature and its `@example`
   doc-comment — name + JSON schema, zero hand-written registration. This is
   what replaces mu's tool registry.
4. **An agent answers end to end.** A `micro` agent on Atlas Cloud discovers the
   tool, calls it, and synthesises an answer from the result — no tool glue.

Sample run (abridged):

```
[1] Direct typed RPC call
   OK Weather for London.
[2] Auto-discovered AI tool (from the method + @example)
   tool "weather_Weather_Forecast"
     schema: {"location":{"type":"string",...},"lat":{"type":"number"},...}
[3] Execute the tool the way an agent does
   result content: {"summary":"Weather for London.\nNow: 16C, overcast..."}
[4] Agent end to end (atlascloud)
   A: Right now in London, it's 16°C and overcast, with humidity 65% and wind 18 km/h.
```

The weather data here is deterministic sample data standing in for
`weather.FetchWeather`, so the spike runs anywhere with no external dependency.

See [`../../GO_MICRO.md`](../../GO_MICRO.md) for the full findings and the
proposed migration path.
