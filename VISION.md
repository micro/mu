# Mu

**A personal server. You own it, it speaks the protocols you already use, and
there is an agent inside it.**

One program. Run `mu --serve` and you have mail on your own domain, messaging
that federates, files, notes, calendar, contacts, a shell, an API, and an agent
that can be reached at an address on any of those. No ads. No tracking. No
company in between. Run it yourself, or let us host it for you.

---

## The problem

The services people depend on every day are owned by a handful of companies who
make their money by watching what you do and selling access to your attention.
That is not a complaint about their engineering. Their engineering is very good.
It is a complaint about the deal: you get the service, they get you.

There are open pieces — a mail server here, a chat server there — but there is
no coherent alternative. Assembling one is a systems administration project, and
the result is worse than what you left. So almost nobody does it, and the ones
who try mostly give up.

What is missing is not another protocol implementation. It is **one thing that
pulls it together**, that an ordinary person can actually run or pay someone
else to run, and that is good enough to prefer.

## Why now

Self-hosting has always lost on the same argument: running your own services is
more work and gives you less. Fewer features, worse apps, no one to call. Owning
your data was a principle you paid for in convenience, and most people, quite
reasonably, would not.

The agent inverts that.

A server with an agent inside it can do things a rented service cannot and will
not. Gmail will not text your plumber. It will not read your mail, check your
calendar, and answer for you. It will not let anything else do those things
either, because your data is the product and an agent with real access to it is
a competitor. Every large provider is structurally prevented from giving you
what an agent needs: everything, in one place, with permission to act.

Your own server has no such conflict. That is the whole opportunity, and it is
new. For the first time, the thing you run yourself can be **better** than the
thing you rent — not merely more principled.

## What it is, and what it is not

Several honest descriptions have been in play. Settling them:

**Not "tools for agents."** That is the API surface, and it is real — the MCP
endpoint works and people use it. But as a description it makes Mu a supplier of
parts to somebody else's product. The tools exist to make the agent useful, not
the other way round.

**Not "a computer."** True, evocative, and it tells nobody what to do on Monday.
Keep it as a way of thinking. Do not put it on the front page.

**Not "a network."** That is the ambition, not the entry point. A network is
worth something at the thousandth user and nothing at the first. Something has
to be worth running when you are the only one running it.

**Yes: a personal server.** That is what it is. One binary, your domain, your
data on your disk.

**Yes: entered through the inbox.** That is how anyone arrives. Everybody
already has an inbox, already resents it, and already understands what one is.
It is the door that needs no explanation.

So: **a personal server, entered through the inbox.**

## How it is built

Four commitments, all of them already true in the code.

**One binary.** `go install` and you have an SMTP server, an IMAP server, an
XMPP server that federates, an MCP endpoint, a REST API, an SSH shell and a web
app. Not a stack to assemble — a program to run. This is the single biggest
reason it can be adopted, and it should never be traded away for the
architectural fashion of the moment.

**Real protocols, not adapters.** Mu *is* the mail server. It *is* the XMPP
server. `you@your.domain` is a real address that real mail reaches, and your JID
federates to other servers. This means the clients already exist: Thunderbird,
Apple Mail, Conversations, Gmail on your phone. We do not have to win a client
war we would lose.

**One record.** Everything that arrives — mail, chat, a text, a WhatsApp message
— lands in the same store, addressable and searchable, whatever door it came
through. The inbox is the view across it, and IMAP is a client of it. This is
what makes an agent able to be useful: it sees the whole of your correspondence,
not one channel's slice.

**Everything has an address.** People, agents, services, conversations. An
address is how a thing is reached and how it is named, and a system where
everything is addressable is one that can be composed. This is already most of
the way true and is worth finishing.

## What we will not do

These are constraints, not preferences. They are the reason the project exists,
and a version of Mu that breaks them is not a compromised Mu, it is a different
product that should have a different name.

- **No advertising.** Ever. The moment attention is the revenue, every design
  decision starts bending away from the person using it.
- **No tracking or profiling.** We do not build a picture of you to sell, rent,
  or "improve the experience" with.
- **No engagement optimisation.** We are not trying to increase time on site.
  Success looks like you spending *less* time here, not more.
- **No lock-in.** AGPL, your data on your disk, standard protocols out. If you
  want to leave, leaving should be easy. If it is not, that is a bug.
- **No dark patterns.** Cancelling is one click. Deleting means deleted.

The reasoning behind these is not commercial positioning. It is that the people
building this will be answerable for what they built — for whether the skills
they were given were used to help people or to extract from them. A tool that
makes someone's life quieter and their obligations easier is worth building. One
that farms them is not, however large it becomes.

## The honest risk

This project has a failure mode and it has already happened once.

Go Micro solved a real problem — standardised service development for a team
scaling a system — and then grew outward into everything, chasing a platform
vision that users had not asked for. The framework was what people wanted. The
rest was motion.

Mu has thirty-five services and a hundred tools. Flight tracking. Food hygiene
ratings. Ordnance Survey map tiles. None of these is wrong, and each one works.
But they are **evidence that the platform is real, not the product itself**, and
it is easy to confuse the two — especially because building another one always
feels like progress.

The services that matter are of two kinds:

1. **The doors** — mail, chat, SMS, WhatsApp. How things get in and out.
2. **The things that are yours** — files, notes, documents, contacts, calendar,
   the record. What you would lose if you left.

Everything else is a demonstration. Demonstrations are worth having, and they
are worth stopping when there are enough of them. There are enough of them.

## What to build next

In order, and short on purpose.

1. **Make the inbox genuinely the whole thing.** Everything that arrives, on
   every channel, in one place, answerable from any client. Mostly done.
2. **Make the agent worth having.** One agent that reliably does ordinary
   errands well beats eleven that mostly work.
3. **Make hosting real.** Someone must be able to pay, get a domain, and have
   this working without touching a terminal. The self-hosting story is the
   principle; the hosted story is the business.
4. **Finish addressing.** Names for entities, resolution from name to whichever
   channel currently reaches them.

Not on this list: more services. Not on this list: more protocols.

## How we will know

Not by user count, and not by funding.

WhatsApp began as a status broadcaster. What made it was Jan Koum noticing that
people in countries with expensive SMS were using the broadcast feature to
message each other — and following that instead of his plan. Uber began because
two people could not get a taxi. Neither started from a vision document. Both
started from someone paying attention to a real irritation and being honest
about what they saw.

So the measure is: **does anyone use this instead of the thing they were using
before, without being asked to care about why?** Not "do they admire it." Not
"do they agree with it." Use it, in preference, because it is better for them.

If the answer is yes for one person who is not us, that is the signal to follow,
whatever it turns out to be about. If the answer is no, no amount of additional
surface will change it, and the right response is to find out what would.

---

*Small, honest, useful, and owned by the people it serves. If it stays that and
never gets big, it was still worth building.*
