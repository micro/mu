# Install Micro

Run Micro on Linux or macOS. On Windows, use WSL2.

## 1. Install

Open a terminal and run:

```sh
curl -fsSL https://micro.mu/install.sh | sh
```

## 2. Start

```sh
mu --serve
```

If your terminal does not recognise `mu`, close and reopen it after installation.

## 3. Open Micro

Go to <http://localhost:8080>. Create your account and choose an AI provider.
You can skip the provider for now; the assistant needs one to answer questions.
A hosted provider needs an API key; a local provider needs a running model.

Keep the terminal open while using Micro. Press **Ctrl+C** to stop; run the same
start command next time. Your data stays in `~/.mu`.

## When you need more

- [Hosting](HOSTING.md) — keep Micro running, use your own domain, update and back up.
- [Configuration](CONFIGURATION.md) — models, service keys and optional features.
- [Mail and messaging](CHANNELS.md) — connect email and XMPP.
- [Architecture and development](ARCHITECTURE.md) — how it works and how to extend it.

Prefer no installation? [Use Micro online](https://micro.mu).
