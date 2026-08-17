# About

**Tools for agents.** The everyday internet — news, mail, search, weather,
markets, video, files, notes — as tools an agent can call over MCP and REST,
paid per request in USDC, with no account in the way.

## One account instead of a hundred

An agent that wants news, mail, search, weather, markets, places and somewhere
to keep records otherwise needs six or seven providers: six signups, six cards
on file, six tokens to rotate. Mu is one balance and one protocol — and for an
agent paying per request, no signup at all.

Removing the barrier is the product. Sometimes that means running the thing
ourselves: the SMTP server with DKIM is real, because a sending domain is not
something you can casually acquire. Sometimes it means paying a provider so the
caller does not have to hold that relationship. Both count. The test is whether
the caller is spared an account.

## Two doors, one set of services

An agent calls `/mcp` and gets the tools. A person signs in and gets a home
screen — a card per service, the agent inline, apps, a wallet. Nothing is built
twice, and a new service appears in both at once.

## Paying

Most of it is free. Storage, and any read that only touches this instance, costs
nothing. What costs anything costs it because a model ran or a paid provider
answered, and the price list is [/tools](/tools) — one file, one credit is
one penny.

There are two ways to pay for the rest, and which one you use depends on whether
you are a person or a program.

**A person tops up with a card.** Sign in, add credits through Stripe, and
priced calls come out of that balance — the web pages, the assistant and any
agent you have given a token all draw on the same one. That is the *one account
instead of a hundred* part: no separate card on file for news, for maps, for
search.

**An agent pays per request in USDC, and never signs up at all.** Every priced
call answers `402 Payment Required` with an [x402](https://x402.org) challenge
naming the price, the asset and where to send it. Pay, retry, get the answer.
No account, no card, no email, nothing to rotate — the payment *is* the
identity. Try it:

```
curl -X POST https://micro.mu/mcp \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call",
       "params":{"name":"web_search","arguments":{"query":"x402"}}}'
```

That comes back 402 with the challenge, today, on this instance.

Self-hosting, neither is switched on. An instance with no Stripe keys and no
x402 address cannot charge anybody, so nothing is metered and every tool is
simply free. Turning payments on is what turns the meter on — see
[Install](/install).

## Yours to run

One Go binary, self-hostable. Run an instance and anyone paying to call your
tools pays you. [Install](/install).

## What is here

[Tools](/tools) is what an agent can call. [Services](/services) is the same
catalogue as things you can open yourself. Both are generated from what this
instance actually runs, so neither can drift from it.
