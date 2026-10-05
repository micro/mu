# Mu

The runtime for **Micro, an open personal assistant**.

## Home

A place to view and do things e.g prompt, see what's happening, etc.

## Inbox

Connect from various places and it all goes in one inbox.

| Protocol | Info |
|---|---|
| HTTP | Start a conversation or reopen one from Inbox. |
| SMTP | Send a message to `agent@your-domain` from your verified email address |
| SMS | Verify your phone number in Account, then text the configured number. |
| XMPP | Connect with your account and a Chat token from Client access, then message the agent. |

## Agents

**Micro** is the default agent. Agents have instructions and a set of
tools. Services provide those tools: mail, files, calendar, search, weather,
notes, shell, and more. You ask for an outcome; the agent chooses the tools.

## Work

Work can run in the background and return its result to the originating conversation. 
Task records live in service/tasks. Agents own the daily brief and delivery logic, 
while service/events schedules it and Work tracks each occurrence.

## Services

Complete standalone services that agents can use as tools or you can browse with. 
For example a complete Mail client and server. Web search via brave. News aggregation, 
headlines and summary via RSS. Video search via YouTube. All behind one MCP interface.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/micro/mu/main/install.sh | sh
~/.local/bin/mu --serve --address 127.0.0.1:8080
```

Open **http://localhost:8080** to create your account and choose an AI provider.

See [Install Micro](docs/INSTALL.md) for the quick start. Optional
[documentation](docs/README.md) covers hosting, configuration, mail and development.

## CLI

The binary acts as a client. It defaults to the hosted instance; set
`MU_URL` or use `mu login https://your.host` for your own server.

```bash
mu ask "What needs my attention?"
mu inbox list
mu help
```

## API

```bash
curl "$MU_URL/agent" \
  -H "Authorization: Bearer $MU_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"What needs my attention?"}'
```

The response includes `text` and `thread`; send the thread identifier
back to continue. Requests and credentials belong in request bodies and headers.
A separately configured x402 host provides paid service calls.
