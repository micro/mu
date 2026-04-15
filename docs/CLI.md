# CLI

`mu` is a registry-driven command-line interface for the Mu platform. It's a thin HTTP client that talks to any Mu instance's `/mcp` endpoint — every MCP tool is automatically available as a subcommand, so adding a new tool on the server side adds a new CLI command for free.

The CLI and the server share the same binary. Running `mu --serve` starts the server exactly as before; running `mu` with anything else is treated as a CLI invocation and never touches server state.

## Install

The same `mu` binary runs the server and the CLI.

```bash
git clone https://github.com/micro/mu
cd mu && go install
```

Or grab a prebuilt binary from the [releases page](https://github.com/micro/mu/releases).

## Quick start

```bash
mu                                      # show help
mu help                                 # live list of all available tools
mu news                                 # latest news feed
mu news_search "ai safety"              # search news
mu chat "hello, what's up?"             # chat with the AI
mu agent "summarise today's markets"    # run the full agent
mu web_search "claude code"
mu weather_forecast --lat 51.5 --lon -0.12
mu apps_search "pomodoro"
mu me                                   # your account (requires login)
```

The first positional argument is always the tool name. The rest are flags that map directly to the tool's parameters.

## Authentication

Most tools work without auth (news, markets, weather, blog reads, etc.). Anything that creates content, spends credits, or reads personal data needs a token.

### Option 1 — browser login

```bash
mu login
```

Opens `/token` in your browser. Sign in to Mu, create a Personal Access Token, paste it back into the terminal. The token is saved to `~/.config/mu/config.json` with mode `0600`.

### Option 2 — paste directly

Works in SSH sessions, containers, or anywhere without a browser.

```bash
mu config set token <TOKEN>
```

### Option 3 — environment variable

For CI, scripts, or ad-hoc use:

```bash
export MU_TOKEN=<TOKEN>
mu me
```

### Logout

```bash
mu logout
```

Clears the stored token. Env vars and `--token` flag overrides are unaffected.

## Pointing at a different instance

By default the CLI talks to `https://mu.xyz`. To point at your own self-hosted instance:

```bash
# Persistent
mu config set url https://mu.example.com

# Per-invocation
mu --url https://mu.example.com news

# Environment
export MU_URL=https://mu.example.com
```

## Passing arguments

Flags map one-to-one with the tool's parameters. Both forms are accepted:

```bash
mu news_search --query "bitcoin"
mu news_search --query=bitcoin
```

For a small set of well-known tools, a single positional argument is treated as the most obvious required parameter, so you can skip the flag name:

```bash
mu chat "hello"                  # same as --prompt "hello"
mu news_search "bitcoin"         # same as --query "bitcoin"
mu web_search "claude code"      # same as --q "claude code"
mu apps_build "a pomodoro timer" # same as --prompt "..."
```

### Types

The CLI infers parameter types from the value:

| Value            | JSON type sent   |
|------------------|------------------|
| `true`, `false`  | boolean          |
| `42`, `-7`       | integer          |
| `3.14`, `-0.12`  | float            |
| anything else    | string           |

If you need to send a string that looks numeric, use `--id=123`. Bare flags (no value) are treated as booleans set to `true`:

```bash
mu apps_create --name "Timer" --slug timer --html "..." --public
```

## Output

### Automatic format

- **Terminal** (default) — pretty-printed, lightly coloured JSON
- **Pipe** — compact JSON, one object per line, so it plays nicely with `jq`

```bash
mu news                        # pretty JSON
mu news | jq '.feed[0].title'  # compact JSON, pipeable
mu news > feed.json            # compact JSON, file-friendly
```

### Forcing a format

```bash
mu --pretty news | less         # force pretty even when piped
mu --raw news                   # force raw even in a terminal
mu --table news_search --query "ai"  # render as a text table
```

`--table` renders list-shaped results as aligned columns, skipping long content fields (`html`, `body`, `content`) to keep the layout readable.

## Global flags

These can appear before or after the tool name:

| Flag              | Purpose                                                  |
|-------------------|----------------------------------------------------------|
| `--url URL`       | Mu instance URL (env: `MU_URL`, default: `https://mu.xyz`) |
| `--token TOKEN`   | Session or PAT token (env: `MU_TOKEN`)                   |
| `--pretty`        | Force pretty-printed output                              |
| `--raw`           | Force raw/compact JSON output                            |
| `--table`         | Render list results as a text table                     |
| `-v`, `--verbose` | Verbose logging                                          |

## Built-in commands

These aren't MCP tools — they're CLI-local commands:

| Command                | What it does                                              |
|------------------------|-----------------------------------------------------------|
| `mu help`              | Fetch the live tool list grouped by app                   |
| `mu help <tool>`       | Show parameters and an example for a specific tool        |
| `mu login`             | Browser-based login (opens `/token`, pastes the PAT back) |
| `mu logout`            | Clear the stored token                                    |
| `mu config get`        | Show the saved URL and whether a token is set             |
| `mu config get <key>`  | Print a single config value (`url` or `token`)            |
| `mu config set <k> <v>`| Save a config value                                      |
| `mu config path`       | Print the path to the config file                         |
| `mu version`           | Show the CLI version                                      |

## Common recipes

### Scripted news digest

```bash
mu news | jq -r '.feed[] | "\(.title)\n  \(.description)\n"'
```

### Weather for a postcode

```bash
mu places_search "EC1A 1BB"    # get lat/lon
mu weather_forecast --lat 51.52 --lon -0.10
```

### Build an app from a prompt

```bash
mu apps_build --prompt "a pomodoro timer with lap counter"
# → returns slug + URL you can open in a browser
```

### Run the agent

```bash
mu agent "find me three interesting AI papers from the last week and summarise them"
```

### Search then tail the first result

```bash
mu web_search "open source self-hosted email" --raw \
  | jq -r '.results[0].url' \
  | xargs -I {} mu web_fetch --url {}
```

## How it works

The CLI is a standalone package (`mu/cli`) with no dependencies on the rest of the Mu codebase. Every invocation:

1. Loads `~/.config/mu/config.json`, applies environment overrides, then flag overrides.
2. Parses the positional arguments as a tool name + `--flag value` pairs.
3. Builds a JSON arguments map with inferred types.
4. Sends a `tools/call` JSON-RPC request to `<url>/mcp`.
5. Formats the response and writes it to stdout.

The same path every MCP client uses. No duplicate code, no drift between CLI and MCP tools — when a new tool is added to the server, it's immediately available on the command line.

## Server deployment

The CLI doesn't affect the server in any way. The binary still launches the server when you pass `--serve`:

```bash
mu --serve --address :8080
```

The dispatch logic looks for `--serve` in the arguments; anything else falls through to the CLI handler, which talks to `/mcp` over HTTP and returns an exit code. Existing server deployments continue to work unchanged.
