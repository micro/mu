# Proving federation works

Sends one message from an account on a Mu instance to a JID on somebody else's
XMPP server, and prints whether it got there.

This exists because outbound federation has exactly one door. A stanza addressed
off-domain arriving on the C2S port is the only thing that reaches `SendRemote`
— `chat_send` posts to rooms, not JIDs — so no `curl` exercises it, and no test
in the repository can without standing up a second XMPP server.

## Run it

The password is a personal access token from [/token](https://micro.mu/token),
the same credential IMAP and submission take.

```bash
export MU_TOKEN=...
go run . -addr micro.mu:5223 -user asim -to someone@jabber.org
```

You need a real account on the far side to see the message land. Without one,
the interesting part still runs: dialback either completes or it does not, and
the difference is visible from this end.

## Reading the result

It stops at the first thing that fails, and only the third is federation:

| | |
|---|---|
| **Timeout connecting** | 5223 is closed on the firewall. Packets never arrived. |
| **Connection refused** | They arrived and nothing was listening — nginx is down, or not on that port. |
| **The instance refused the token** | The token is wrong, revoked, or belongs to another instance. |
| **`remote-server-not-found`** | Dialback did not complete. This is the federation failure. |
| **Nothing at all** | It worked. The instance took the message and the remote server accepted it. |

Silence being success is why it waits after sending: a stanza error is the only
thing that comes back, and dialback against a slow domain can take most of the
ten-second dial timeout before producing one.

When it is `remote-server-not-found`, the instance's log under `chat` says which
half failed — an outbound dial that never connected, or a verification call that
came back invalid. The first is usually egress: dialback means this instance
dials *out* to every domain it talks to, so a security group that only allows 80
and 443 outbound fails every handshake while looking like a broken peer.

## Why there is no XMPP library here

The opposite choice from [`../imap-client`](../imap-client), for the opposite
reason. There the client *is* the test: Mu's IMAP server is hand-written, so
speaking to it through a library the rest of the Go mail world uses is the whole
point. Here the thing under test is a handshake between two servers that this
program never touches, and four stanzas written out by hand keep it that way —
nothing in here can be quietly doing the work instead.
