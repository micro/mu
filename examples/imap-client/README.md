# Reading a Mu mailbox over IMAP

A small IMAP client that signs in to a Mu instance, lists its folders, prints
the newest messages and — with `-watch` — sits on IDLE waiting for the next one.

It is the interop test as a program. Mu's IMAP server is hand-written and has no
dependency (`service/mail/imap.go`), so it has only ever been spoken to by its
own tests, which is a server agreeing with itself. This talks to it through
[emersion/go-imap](https://github.com/emersion/go-imap), which most of the Go
mail world uses and which has never heard of Mu.

If this works, Thunderbird works. That is the whole point of it.

## Run it

The password is a personal access token, minted at
[/token](https://micro.mu/token). Mu has no password to type into a mail client
— sign-in is a passkey or a link — so a token stands in. It is the app-password
pattern, and it is revocable without touching how you sign in.

```bash
export MU_TOKEN=...                    # a personal access token
go run . -user asim -addr localhost:1143
```

```
connected to localhost:1143 as asim

folders:
  INBOX
  INBOX/research
  Junk

INBOX: 3 messages, 3 unseen, uidvalidity 1787257236, uidnext 4

  uid 1      •  sender@example.com           Invoice 4021
  uid 2      •  sender@example.com           Three papers on retrieval
  uid 3      •  sender@example.com           Café — a non-ASCII subject ☕
```

A `•` is unread. The last line is there on purpose: a subject that is not ASCII
is [RFC 2047](https://datatracker.ietf.org/doc/html/rfc2047) encoded on the
wire, and whether the server encoded it correctly is not something the server
can tell you — the client decoding it is the only proof.

Against an instance with TLS in front of it, on the port the terminator offers:

```bash
go run . -tls -user asim -addr mail.example.com:993
```

And to watch. It holds IDLE open and prints when the count changes, which is
what makes a mail client feel like one — the server says when something arrives
rather than being asked every minute.

```bash
go run . -user asim -watch
```

## Flags

| Flag | Default | |
|---|---|---|
| `-addr` | `localhost:1143` | host:port of the IMAP listener |
| `-user` | | your account name, or the full address |
| `-tls` | `false` | connect with TLS — see below |
| `-n` | `10` | how many of the newest messages to list |
| `-watch` | `false` | wait for new mail with IDLE |

## Folders are tags

Mu has no folders to create. A folder here is a plus-address tag: mail sent to
`you+research@` shows up as `INBOX/research`, and `Junk` is where the spam
filter's decisions are visible so you can disagree with them. Both are derived
from what has arrived, which is why the server answers `NO` to `CREATE`,
`RENAME`, `DELETE` and `APPEND` rather than pretending — and does not claim them
in its capability list either.

Every message is also in `INBOX`, so a client that syncs one folder gets the
whole mailbox. UIDs are per folder, which is what the protocol expects.

## TLS

The server listens in the clear, deliberately: nothing in this repo terminates
TLS — the web server runs behind a proxy that does — and the operator puts the
same terminator in front of the IMAP port on 993. `-tls` is for connecting to
that, not to the plaintext port. See
[docs/INSTALL.md](../../docs/INSTALL.md#reading-your-mail-in-a-mail-client).

## Running an instance to point it at

```bash
go build -o mu . && ./mu --serve
```

IMAP is on by default on `:1143` — a high port, because an unprivileged process
cannot bind 143 and a default that cannot start is a feature nobody finds.
`IMAP_PORT=143` in production, `IMAP_PORT=off` to have none.

## Any client, not just this one

Nothing here is Mu-specific beyond the address and the token. The same settings
work in Thunderbird, Apple Mail or `mutt`:

| | |
|---|---|
| Server | your instance, port `143` (or `993` with TLS) |
| Username | your account name |
| Password | a personal access token from `/token` |
