# Mu CLI

`mu-cli` is a standalone terminal client for a Mu runtime. It uses MUCP over
HTTPS and has no server, model-provider or protobuf dependencies. Agents, tools,
history, permissions and billing remain on the chosen server.

## Start

Build with Go 1.26.4 or newer:

```sh
cd client/cli
go build -o mu-cli .
./mu-cli login
./mu-cli ask "What should I focus on today?"
./mu-cli chat
```

Release builds provide `mu-cli-<os>-<arch>` for Linux, macOS and Windows
(`.exe` on Windows). Rename the downloaded executable to `mu-cli` and put it on
your PATH; on Linux/macOS, make it executable with `chmod +x mu-cli`.

The default server is micro.mu. Use `mu-cli --server https://your-server login`
to connect elsewhere. Browser sign-in uses OAuth with PKCE; the browser must be
on the same computer. For headless use, set `MU_TOKEN` or pipe an API token into
`mu-cli login --token-stdin`. Tokens need the scopes for the requested operations.

Saved credentials live in your OS user configuration directory under
`mu-cli/session.json`. This is a private file (0600 on Unix), not encrypted
storage. OAuth sessions renew automatically; `MU_TOKEN` avoids credential storage.
`logout` removes the local session; revoke tokens in the server account settings
to invalidate them. If a process crashes while updating credentials, remove its
`session.json.lock` only after all `mu-cli` processes have stopped.

## Use

```sh
mu-cli agents
mu-cli threads
mu-cli read THREAD_ID
mu-cli --agent AGENT_ID --thread THREAD_ID chat
mu-cli home
mu-cli inbox
mu-cli services
mu-cli call assistant Home.Overview '{}'
```

Put flags before the command. Lists and reads return JSON, including pagination
tokens; pass `--page TOKEN` for another page. Answers print text and content
fallbacks; `--json` returns the complete operation. In chat, `/new` starts a new
conversation and `/quit` exits. Use `ask -` or `call SERVICE ENDPOINT -` to read
private input from stdin instead of placing it in shell history.

Requests are submitted once with a message ID printed to stderr. While a request
is running, the client checks its operation every two seconds; there are no idle
or background requests. Ctrl-C stops waiting, not server execution. Resume an
accepted request with `mu-cli operation OPERATION_ID`. If submission was not
confirmed, check Chats or retry the identical prompt, server, agent and thread
with `--message-id ORIGINAL_ID` to avoid duplicate execution.

`mu` retains runtime startup and operator commands. Existing remote-use commands
remain available during migration; `mu-cli` does not start a local runtime.

## Development

Run `go test -race ./...`, `go vet ./...` and `gofmt -l .` in this directory.
This is a separate Go module, so the root module's tests do not include it.
The implementation is covered by the repository's AGPL licence.
