# JMAP mailbox access

Micro exposes experimental JMAP access through Mail, alongside IMAP. Discovery
is `/.well-known/jmap`; the API is `/mail/jmap`. Use HTTPS in production, your
Micro username (or mail address) and a mail access token as the Basic-auth
password. Bearer mail tokens also work. Browser cookies do not authenticate this
protocol. Account and token scope checks apply to every request; read-only
service tokens cannot mutate messages.

Implemented:

- Session discovery, Core/echo, batched calls and result references.
- Mailbox/get, Email/get, Thread/get, Email/query with common filters, sorting,
  pagination and collapsed threads.
- Email/set read/unread changes and deletion using Mail's existing operations.
- Authenticated raw-message, body and attachment downloads.
- State hashes and event-source notifications; changed snapshots return
  cannotCalculateChanges so clients perform a full resync.

The folder view and bridged conversations match IMAP. Ordinary Mail read state
is shared; bridged-message flags use the existing mail-client projection and do
not change the source conversation. Deletion follows the same ownership rules
as IMAP, and bridged deletion only hides the mail projection.

This is not yet full mail-client support: drafts, submission, uploads, imports,
copying/moving messages and mailbox mutations are unavailable. Unsupported
operations return explicit errors. The submission capability is not advertised.
Use IMAP plus SMTP submission for a full mail client. Lttrs has not yet been
verified against this implementation.

Requests are limited to 1 MiB, 32 method calls, 500 fetched objects, 100 mutations,
four simultaneous requests per account and 32 globally. Streaming connections
reconnect after two minutes. Response generation rejects message selections
whose combined stored bodies exceed 16 MiB; fetch smaller batches.
