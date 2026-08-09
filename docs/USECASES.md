# What you can build

Mu is tools for agents. This is the other half of that sentence: what the
agents are.

The tool list is not what decides it. Anyone can wrap a search API. What
decides what you can build here is **what can wake an agent up**, because an
agent is a trigger, a set of tools, and somewhere to put the answer. Two of
those three are the interesting ones.

## What can start an agent

| Trigger | How |
|---|---|
| **A schedule** | `events_create` with a `prompt` and a `repeat`. When it comes due the agent runs and the answer is mailed to you. |
| **A message** | Ask it here, or from Discord, Telegram or WhatsApp. |
| **Mail** | Write to `you+agent@` and that agent answers. |
| **Another agent** | It calls in over MCP with a scoped token. |
| **You** | Hand it a task on `/tasks` and press run. |

The first and third are the ones worth building around, and they are the two a
stdio MCP server cannot have. A process that only exists while a client is
attached has nowhere to put "every morning, brief me" and no address for anyone
to write to. Mu stays up, runs a real SMTP server with DKIM, and keeps your
records — so an agent here can act while nobody is watching and be reachable
when you are not.

That is the line worth holding on to: **Mu is where an agent lives when nobody
is watching.**

## Things that work because the server stays up

### A morning brief

The one everybody wants and nobody can run from a laptop.

```
events_create  repeat=daily, at 07:00, prompt="What do I need to know today?"
   → news_list · markets_list · weather_forecast · events_list · mail_inbox
   → mailed to you
```

Every piece exists. The agent decides which tools the question needs; you wrote
one sentence.

### A watcher with a memory

The difference between "what is the price" and "tell me when it moves".

```
events_create  repeat=hourly, prompt="Check X. If it changed since last time, tell me. Otherwise say nothing."
   → markets_list (or news_search, or web_fetch)
   → db_get   what it was last time
   → db_create the new state
   → mail_send only if it crossed the line
```

`db` is what makes this an agent rather than a query. A conversation forgets;
a collection does not.

### A digest of your own things

```
events_create  repeat=weekly, prompt="Summarise what came in this week."
   → index_search across your mail, notes and files
   → blog_create or files_put
   → a URL you can send to someone
```

`index_search` reaches your own content, so this is a summary of your week
rather than of the internet's.

## Things that work because we run the mail server

Every agent has an address: `you+name@` reaches the agent called `name` on your
account. It can be written to by anyone you have corresponded with, and by your
own verified address.

### An agent you can email

```
someone writes to  you+research@
   → the agent reads it, with its own standing instruction and its own scope
   → contacts_find · web_search · files_put · whatever the question needs
   → mail_send replies to them
```

Intake, triage, a research assistant you cc. The reply comes from your domain,
signed with DKIM, in the thread.

### An agent that writes to people

```
"find me a slot with Sam next week and offer it"
   → events_free   when you are actually free
   → contacts_find Sam's address
   → mail_send     three options
   → events_create when they pick one
```

## Things that work because a token can be narrow

An agent is a name, a standing instruction, and the services it may reach. The
scope is enforced at the MCP boundary against its token, not merely displayed.

### An agent you can hand to someone else

Scope one to `news` and `web`, issue its token, and give it to a colleague to
point Claude Desktop at. They get those tools on your instance. They do not get
your mail, your files, or your records, because the token cannot reach them.

### An agent per piece of work

One scoped to `db` + `files` + `index` for a project; one scoped to `news` +
`web` for reading. Each has its own instructions and its own runs, so "what has
this one been doing" is a question with an answer — `/agent/runs?agent=…`.

## Things that work because there are payment rails

### A tool endpoint other people's agents pay for

Run an instance, and an agent that is not yours can call your tools by paying
per request in USDC over x402 — no account, no signup, no invoice. The
`accepts` block in the 402 response tells it the price; it pays and the call
proceeds.

That is the self-hosting business in one paragraph: anyone paying to call your
tools pays you.

## Where to start

If you are new here, build these in this order. Each one takes a sentence.

1. **A morning brief.** Proves the schedule.
2. **Something you can email.** Proves the address.
3. **A watcher.** Proves the memory.

By the third you have an agent that runs on its own, can be reached by people
who are not you, and remembers what it saw last time. That is not a chat
window with tools bolted on, and it is not something you can assemble out of a
tool list.
