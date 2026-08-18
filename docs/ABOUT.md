# About

**A personal agent.** It has an email address; write to it and it answers —
in the thread, remembering the last one. Behind it is the everyday internet:
news, mail, search, weather, markets, video, files, notes, as tools it can call
over MCP and REST, paid per request in USDC, with no account in the way.

## An address is the smallest interface

Every agent here has one. `agent@` reaches the default; your own get their own,
`you+research@`, and each answers where it was written to.

An address needs nothing on the other side. No SDK, no OAuth, no protocol to
adopt, no account with us. A person can write to one from their phone, another
agent can write to one from anywhere, a form or a cron job can write to one at
three in the morning. That is what makes an agent something you have rather than
something you visit — it is reachable whether or not anybody has a page open.

Email is the channel we lead with because it is the only one that costs nobody
anything to start using. The inbox itself is not email: it is every conversation
this account has had with an agent, on whichever client it arrived — the web,
the address, Discord, Telegram, WhatsApp — kept in one record and read in one
place. New channels join that record rather than starting a second one.

The other half of the story is that these are not toys. An agent that can be
written to and cannot do anything is a chatbot with an address, so what is
behind it matters:

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

An agent calls `/mcp` and gets the tools — and the inbox with them, because
`recall_list`, `recall_conversation` and `recall_search` are the conversation
list, one thread read back, and search across all of them. Something connecting
from Claude or Cursor reads what arrived here rather than only calling tools.

A person signs in and gets the inbox, a home screen with a card per service, the
agent inline, apps and a balance. Nothing is built twice, and a new service
appears in both at once.

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
