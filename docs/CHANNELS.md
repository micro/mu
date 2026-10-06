# Mail and messaging

Optional setup for your own mail domain, mail clients and XMPP.
You do not need these steps to use Micro in a browser.
See [Hosting](HOSTING.md) for TLS and server deployment.

## Operator inbox

`admin` is a permanent built-in system identity. It owns operational mail and
cannot sign in, hold API tokens, be edited or be deleted. Human administrators
use their own accounts to access **Admin → Mail** (`/admin/mail`). It does not
count as a human administrator during first-run setup.

Mail to `admin@your-domain`, `support@`, `postmaster@`, `abuse@` and `security@`
shares that inbox. The aliases become tags; `admin+dmarc@your-domain` uses the
`dmarc` tag. These are addresses, not additional accounts or agents. Existing
inbound-mail restrictions and spam filtering still apply; operator mail never
triggers an automatic agent response. Additional reporting providers may need
to be allowed through the inbound filter at `/admin/spam`.

The DMARC tab summarises original stored report attachments and deduplicates
reporter/domain/report-ID/date-range combinations. The original messages and
attachments remain accessible in the inbox. For this domain, change only the
`rua` portion of its DMARC TXT record to `mailto:admin+dmarc@your-domain`.
Reporting for a different organisational domain also requires external-report
DNS authorisation at the receiving domain.

## Mail

To send and receive as your own domain:

1. **MX record** pointing at your server.
2. **Port 25** inbound — or `MAIL_PORT=2525` for testing.
3. **DKIM keys**, so your mail is signed and not treated as spam:

```bash
./scripts/generate-dkim-keys.sh
```

That prints a `DKIM_PRIVATE_KEY` for your environment and a TXT record to add at
`<selector>._domainkey.<your-domain>`, where the selector is `MAIL_SELECTOR`
(default `default`).

4. **SPF** — a TXT record at your domain authorising your server to send.

Set `MAIL_DOMAIN` to the domain and restart. `mail_send` is account-only: an
unauthenticated caller can never send, so a paying agent cannot spend your
domain's reputation.

### Reading your mail in a mail client

Mu speaks IMAP, so the mail this instance receives can be read in whatever
client is already open — Mail.app, Thunderbird, your phone — and the agent's
replies appear in the thread there.

| | |
|---|---|
| Server | your domain |
| Incoming (IMAP) | `IMAP_PORT`, `1143` by default; set it to `143` in production |
| Outgoing (SMTP) | `SUBMISSION_PORT`, `1587` by default; set it to `587` in production |
| Username | your Mu username, or your full address |
| Password | an app password from `/account/tokens` |

Signed in, `/inbox/imap` says all of this filled in for the account reading it.
Set `IMAP_PUBLIC` and `SUBMISSION_PUBLIC` to `host:port` if what you put in
front of these listeners answers somewhere other than the defaults below.

Create a mail app password in Clients. Use the same app password for IMAP and SMTP. Revoking it disconnects that app without changing your account password.

**Outgoing is a separate listener from the MTA.** `MAIL_PORT` is the server that
receives mail from the internet and authenticates nobody, which is what port 25
is for. `SUBMISSION_PORT` is where *you* send from, and it authenticates
everybody: nothing happens on it before AUTH, and the address in `From` must be
one your account owns, so a token is not a way to send as somebody else.

What goes out through it is the same mail the compose form sends — same
allowance, same price, same rules about who you may write to. See
`service/mail/outbound.go`, which is the only way mail leaves an instance.

**Folders are your addresses.** The inbox holds everything. Each plus-address
tag you have received mail at is a folder of its own — mail to `you+research@`
appears in the folder *INBOX/research* — so an agent's mail can be subscribed to
on its own. *Junk* is what the spam filter caught, where you can see it and
disagree with it.

**TLS is the proxy's job.** Nothing in Mu terminates TLS; the web server runs
behind something that does, and IMAP is the same. Bind the listener to loopback
so only the proxy can reach it, and never expose the plaintext port — a token
would cross it in the clear.

```
IMAP_PORT=127.0.0.1:1143
```

nginx does this with the **stream** module, not the mail one. `ngx_mail` speaks
IMAP itself and wants an `auth_http` endpoint to tell it which backend to use
for each user; Mu authenticates its own sessions, so there is nothing for that
endpoint to decide. `ngx_stream` is a TCP proxy with TLS on the front, which is
exactly the missing piece.

```nginx
# At the TOP LEVEL of nginx.conf — a sibling of http {}, not inside it.
# conf.d/*.conf and sites-enabled/* are both included from within http {},
# so a stream block dropped there fails to load.
stream {
    upstream mu_imap {
        server 127.0.0.1:1143;
    }
    upstream mu_submission {
        server 127.0.0.1:1587;
    }

    server {
        # Both, on a host with an AAAA record. A stream block takes no IPv6
        # listener by default, so `listen 993 ssl` alone binds 0.0.0.0 and a
        # client that resolves AAAA finds nothing. The web server does not
        # show this because the packaged default site carries a listen [::]
        # line of its own.
        listen 993 ssl;
        listen [::]:993 ssl;

        # fullchain.pem, not cert.pem. A browser will fetch a missing
        # intermediate and a mail client will not, so half a chain is a site
        # that works in Firefox and fails in Gmail with the same message.
        ssl_certificate     /etc/letsencrypt/live/your-domain.com/fullchain.pem;
        ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;
        ssl_protocols       TLSv1.2 TLSv1.3;

        proxy_pass    mu_imap;

        # Longer than the server's own 30-minute idle timeout. nginx defaults
        # to 10 minutes here, which silently drops every client sitting on
        # IDLE — the mail arrives and nobody is told.
        proxy_timeout 35m;
    }

    # Outgoing, so the client can reply. 465 is implicit TLS, the same as 993:
    # this listener offers no STARTTLS, so a client told to use it on 587 would
    # send the token in the clear believing otherwise.
    server {
        listen 465 ssl;
        listen [::]:465 ssl;

        ssl_certificate     /etc/letsencrypt/live/your-domain.com/fullchain.pem;
        ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;
        ssl_protocols       TLSv1.2 TLSv1.3;

        proxy_pass    mu_submission;
        proxy_timeout 5m;
    }
}
```

The same certificate as the web server. On Debian and Ubuntu the module is a
separate package (`apt install libnginx-mod-stream`); elsewhere nginx needs
`--with-stream --with-stream_ssl_module`, which `nginx -V` will tell you.

### XMPP goes in the same block

`XMPP_PORT` is the same arrangement, one more `server` inside the same
`stream {}`. 5223 is XMPP's implicit-TLS port — the direct-TLS one, the same
idea as 993 and 465 — and it is what a modern client tries first.

```
XMPP_PORT=127.0.0.1:5222
```

```nginx
    upstream mu_xmpp {
        server 127.0.0.1:5222;
    }

    server {
        listen 5223 ssl;
        listen [::]:5223 ssl;

        ssl_certificate     /etc/letsencrypt/live/your-domain.com/fullchain.pem;
        ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;
        ssl_protocols       TLSv1.2 TLSv1.3;

        proxy_pass    mu_xmpp;

        # A chat connection is idle most of the time and closing it is a
        # reconnect and a re-auth. The server's own read deadline is 10
        # minutes; nginx must be longer or it is the one hanging up.
        proxy_timeout 15m;
    }
```

There is no STARTTLS on 5222, for the same reason there is none on 143: the
server does not advertise it, so a client told to use it there would be sending
a token in the clear believing otherwise. Bind it to loopback and let 5223 be
the only way in.

**One DNS record makes it findable.** A client given `you@your-domain.com` has
only the domain to go on, and looks up SRV records before it tries anything.
Without one it guesses the domain itself on 5222, which is not where this is.

```
_xmpps-client._tcp.your-domain.com. 3600 IN SRV 5 0 5223 your-domain.com.
```

`_xmpps-client` is direct TLS (XEP-0368) — the record for 5223. **Do not also
publish `_xmpp-client._tcp` at 5222.** That is the STARTTLS record, 5222 is
bound to loopback, and a record pointing at a closed port is worse than no
record: the client tries it, waits, and reports a timeout rather than telling
anybody what is wrong.

The cost is that a client too old to do direct TLS cannot connect at all. That
is the same trade IMAP makes on 143 and it is the right way round — a client
that cannot do TLS properly should fail to connect rather than succeed in the
clear. Conversations, Dino, Gajim and Monal all do direct TLS.

The target needs an A record, which the domain already has: it is the web
server. And open 5223 on the firewall — see *Check it from somewhere else*
below, which applies here unchanged.

### XMPP federation does not go through nginx

`XMPP_S2S_PORT` is 5269 and it faces the internet directly. Open the port on
the firewall; there is no `server {}` block to add.

The reason is the same one that makes federation deployable at all. A server
connecting here proves which domain it is by dialback, not by its certificate:
it hands over a key, this instance opens its own connection to the domain being
claimed and asks whether the key is theirs, and the answer arrives over a link
to whatever address DNS gave for that domain. So the certificate on 5269 is not
what establishes identity — every federated server skips verifying it, and this
one offers a self-signed certificate generated on first use. Putting nginx in
front would terminate a TLS session whose certificate nobody checks, add a hop,
and make every peer's source address `127.0.0.1`.

**One DNS record, and it is optional.**

```
_xmpp-server._tcp.your-domain.com. 3600 IN SRV 5 0 5269 your-domain.com.
```

Optional because a server with no SRV record to go on falls back to the domain
itself on 5269, and the domain already has an A record — it is the web server.
Publish it anyway if the XMPP host is ever going to be somewhere other than the
web host, which is the whole point of the indirection; it is the same record MX
is for mail. Priority and weight do nothing with one target — the 5 and the 0
match the client record above so the two read the same.

What does matter is that `your-domain.com` — or the SRV target, if you publish
one — resolves to this host on 5269 from the outside. That is where other
servers connect *and* where the dialback verification call comes back to, so a
name that resolves somewhere else fails the handshake rather than the message.

To check the port from somewhere else, open a stream at it. A working listener
answers with its own stream header naming your domain:

```bash
printf "<?xml version='1.0'?><stream:stream xmlns='jabber:server' xmlns:stream='http://etherx.jabber.org/streams' xmlns:db='jabber:server:dialback' to='your-domain.com' version='1.0'>" | nc your-domain.com 5269
```

That checks the port is open, which is not the same as federation working.
For that, **/admin/diagnostics** has a Federation check with a link that dials
`jabber.org` and completes a real dialback handshake — SRV lookup, outbound
dial, and their verification call arriving back here. It needs no account and
no recipient, because dialback does not: proving a domain is a conversation
between two servers, and nobody has to be listening at either end of it.

Outbound 5269 has to be open too, and it is the half that gets forgotten:
dialback means this instance dials *out* to every domain that connects to it.
Egress is usually unrestricted, but a locked-down security group that only
allows 80 and 443 out will accept federated connections and then fail every one
of them at verification, which reads like a broken peer rather than a firewall.

### SSH does not go through nginx

`SHELL_SSH_PORT` is the exception, and it is worth being explicit about because
the instinct is to put everything behind the one proxy.

SSH carries its own transport encryption and does its own host-key
verification. There is no TLS to terminate and no virtual host to pick, so
nginx would be a plain TCP forwarder adding a hop, a second timeout to get
wrong, and a source address that is `127.0.0.1` for every session. Open the
port instead:

```
SHELL_SSH_PORT=2222
```

Wrapping it in `stream { server { listen 2222 ssl; ... } }` is the one thing
that is actively wrong — that offers TLS on the front, and an SSH client does
not speak TLS, so every connection fails at the handshake with a message about
neither protocol.

Pick a port other than 22. That one is the host's own `sshd`, and taking it by
accident locks you out of your own machine.

This is 993, implicit TLS — the port every client offers first. There is no
STARTTLS on 143: the server does not advertise it, so a client asked to use it
there would be sending a token in the clear believing otherwise.

**Check it from somewhere else.** Every test run on the server itself passes
while the port is unreachable from the internet, which is how an afternoon
goes. `ss -lntp` showing `0.0.0.0:993` means nginx is bound, and nothing more.

What the failure looks like tells you where it is. *Connection refused*, at
once, means the packets arrived and nothing was listening — nginx is not up, or
not on that port. *Nothing at all*, until it times out, means they were dropped
before they got there: a firewall. A mail client reports both as the same
unhelpful sentence, so the distinction has to come from `openssl`.

Cloud firewalls are the usual culprit, because they are default-deny and
typically opened for 80, 443 and 22 alone — DigitalOcean's Cloud Firewalls
drop rather than reject, so they produce the timeout above. Check the
provider's rules and the host's own (`ufw status`, `iptables -L INPUT -n`):
having both, with only one of them open, is easy to do.

To check the whole path:

```bash
printf 'a1 LOGIN you TOKEN\r\na2 LOGOUT\r\n' | openssl s_client -quiet -crlf -connect your-domain.com:993
```

`stunnel` and Traefik do the same job if nginx is not what is in front.

Folders cannot be created, renamed or deleted from the client, and a client
cannot upload mail into one. Folders here follow your addresses and your mail,
so there is nothing for those commands to do that would still be true a minute
later.

### Outbound deliverability

By default Mu delivers its own mail: it looks up the recipient's MX and speaks
SMTP to it. That is correct and it is not the hard part. The hard part is the
reputation of the IP the packets came from — a new address with no history, no
feedback loop and no bounce processing gets filed as spam by the large providers
however carefully the message is signed, and nothing in the protocol fixes it
from this end.

So outbound can go through a submission server instead:

```bash
export SMTP_RELAY_HOST="smtp.provider.example"   # :587 assumed
export SMTP_RELAY_USER="apikey"
export SMTP_RELAY_PASS="..."
```

Anything that speaks submission works — this is named for the protocol, not for
a provider. The message is still built here and still signed with your own DKIM
key; the relay is one hop, not a rewrite. STARTTLS is required, because the
credential crosses that connection.

Inbound is unchanged either way: Mu runs its own SMTP server and owns the
mailbox, which is the half that matters.

### Who is allowed to send you mail

This instance does not accept mail from strangers. A message gets in if **any
one** of these is true:

| | Rule |
|---|---|
| 1 | It is a reply to something you sent — `In-Reply-To` or `References` matches a Message-ID this server generated. |
| 2 | You have written to that address before. Recorded automatically on the way out. |
| 3 | The sender's domain is whitelisted — see below. |
| 4 | The sender's address is verified on an account here. Somebody who proved they own a mailbox is not a stranger, whatever their domain. |

Anything else is refused with a `550`, so the sender's own mail server tells
them rather than the message disappearing.

**Building your own whitelist.** Set `MAIL_WHITELIST` to a comma-separated list
of domains:

```
MAIL_WHITELIST=acme.com, partner.co.uk, supplier.example
```

It is live — change it at `/admin/config` and the next message is judged by the new
list, no restart. There is also a built-in list of common company and
infrastructure domains. Consumer domains (`gmail.com`, `outlook.com`,
`hotmail.com`) are deliberately **not** on it: they are where unsolicited mail
comes from, and rule 4 already covers the case that matters — your own users
writing in from a personal address.

There used to be a fifth rule: mail addressed to `support@` and nothing else got
through whatever the sender's domain, because the point of a support address is
hearing from people you have never heard of. That also made it the one address
here that spam could reach, and a per-sender cap does nothing about a thousand
senders. The address, the page and the rule are gone.

## ActivityPub (optional)

Separate from the XMPP federation above, and a different network: this one
publishes blog posts to Mastodon and the rest of the fediverse.

Set `MU_DOMAIN` to your public domain and blog posts federate over ActivityPub —
remote servers resolve your users at `/.well-known/webfinger` and actor URLs
under that domain. It must match the domain you actually serve on.

## Incoming messages and assistant access

Receiving a message does not give its sender permission to use an account's assistant.

- Web and authenticated mail clients use the signed-in account.
- SMS and WhatsApp require a valid provider signature and a phone number verified by the account owner before the assistant runs. An outgoing conversation permits replies, not assistant access.
- Email to `agent@<domain>` runs the sender's own assistant only when the visible sender is authenticated and the email address is verified on that account.
- Personal mail addresses, including `username+label@<domain>`, receive correspondence. The plus label is a mail tag/filter, not an agent selector or an instruction to execute. Adding a sender to contacts does not authorise them to run an assistant.
- New senders contacting the shared assistant address receive a fixed registration message. They must create an account or sign in, verify their address or phone, then resend their request. No account is created silently, and the original request is not executed or filed under the operator.
- Automatic emails, mailing lists, bounces and unauthenticated senders do not receive registration replies. Mail registration replies do not create an outbound allowlist relationship. Email replies are limited to one successful welcome per sender per UTC day, within 200 send attempts per instance per day; failed deliveries use that budget too. The budget survives restarts and does not use visitor IP addresses.

Aliases are receive-only by default. User-defined actions such as notifying, drafting a reply or replying automatically are not implemented; they would require an explicit account-owned rule and scope. Existing agent creation routes are separate from mail aliases and are not removed by this policy.

### OMEMO in Conversations

With a Micro account added to Conversations using a Chat (XMPP) token, messages
with `agent@<your domain>` support legacy OMEMO
(`eu.siacs.conversations.axolotl`, the version used by Conversations). Keep OMEMO
enabled. The server publishes device lists and bundles, persists Micro's identity
and ratchet state, and encrypts replies and their XMPP archive entries. Replies
remain encrypted when the phone reconnects. The token's connection settings show
Micro's fingerprint for comparison with Conversations.

Micro is the receiving endpoint: it decrypts requests for assistant processing.
This is not zero-knowledge hosting, encrypted-at-rest storage, or encryption
against the model provider. Plaintext assistant context remains account-scoped.
Keys are stored in `data/chat/omemo.json` with the same restrictive file permissions
as other credentials. Preserve this file during deployment and backup; do not
regenerate it on restart. Changing the XMPP domain requires explicit key migration.

Direct OMEMO chats support a local Micro account talking to `agent@<your domain>`.
Direct OMEMO forwarding between people, encrypted federation, and OMEMO 2 are not implemented;
unsupported encrypted messages are rejected, never interpreted as plaintext.
Device key changes under an existing device ID are refused. New devices publish
a new ID; Conversations controls the user's trust in Micro's fingerprint. Once
an account has established OMEMO with Micro, plain messages in that chat are
refused and encryption failures never fall back to a plain reply.

The optional interoperability check uses the same Signal library as Conversations
(2.6.2), with a Java 17+ source launcher and independently implemented AES-GCM.
Set `OMEMO_JAVA_CLASSPATH` to jars for `signal-protocol-java:2.6.2`,
`curve25519-java:0.4.1`, `protobuf-java:2.5.0`, and `gson:2.11.0`, then run
`go test ./service/chat -run OMEMO -count=1`. Without that variable, the Go-only
OMEMO checks still run and the Java interoperability check is skipped.


### Private groups and XMPP rooms

Open **Services → Groups** (`/groups`) to create a group and invite existing local
Micro usernames. Invitations expire after seven days and grant no access until
accepted on the recipient's Groups page. Owners can promote admins, transfer
ownership, remove members and delete the group. Admins can invite and remove
ordinary members. An owner must transfer ownership before leaving.

Each group has one persistent private chat at `/chat?id=group_<opaque group ID>`.
The same room is available in Conversations at `<group ID>@groups.<your domain>`;
use **Open in XMPP** from its chat page or the client's discovered group service.
The authenticated local XMPP connection routes this component internally, so no
extra listener or DNS record is needed for these local-account rooms. Create
rooms and manage invitations in Micro; arbitrary XMPP room creation, external
members, calls, and XMPP room administration are not supported in this version.
Membership is checked on web, API and XMPP reads and sends, and before broadcasts.
Leaving or removal revokes future server access; it cannot erase copies already
received by a member's client.

Choose the chat mode when creating a group:

- **Web and XMPP chat:** ordinary private chat, readable and writable in Micro's
  own UI and XMPP clients. Transport encryption does not hide stored messages
  from the server. This mode rejects OMEMO payloads rather than displaying an
  unreadable mixed transcript.
- **End-to-end encrypted:** XMPP clients exchange legacy OMEMO envelopes. Micro
  stores and relays ciphertext without decrypting it. Real member JIDs are
  visible to group members for device discovery. Micro's web page manages the
  group and displays encrypted-message placeholders; it cannot decrypt or send
  to these rooms. Plaintext messages are refused. Newly invited members cannot
  decrypt earlier messages that were not encrypted for their devices.

Group history is persisted across restarts. XMPP joins replay the latest 20
messages; clients can retrieve earlier messages through room MAM archive paging. Personal
notes, tasks, calendars and assistant conversations are not shared by joining a
group, and group messages do not trigger Micro or use a member's private context.
Group membership is stored atomically in `data/groups.json`; include it alongside
room records in backups. An unreadable membership file fails closed.
