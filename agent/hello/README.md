# Hello

`hello@<MAIL_DOMAIN>` introduces Micro without creating an account. The built-in
Hello agent uses a lightweight provider model, no tools and at most 400 output
tokens. `HELLO_MODEL` can explicitly override the model; OpenRouter defaults to
GLM Flash, other providers use their configured background-model selection.

Each authenticated sender gets three replies for the lifetime of the trial.
The third includes the signup invitation. There is a shared limit of 100 new
introductory messages per UTC day. Additional messages after the third do not
invoke a model. Sender allowance and daily reservations persist across restarts.

SMTP accepts the message only after persisting it. A single worker processes the
queue. Model attempts are reserved before execution; relay retries reuse the
saved answer and stable Message-ID, with three relay attempts. Automatic mail,
unverified sender claims and mixed-recipient deliveries cannot start a trial.
Attachments are not given to the model. No accounts, credentials, wallets or
outbound-whitelist relationships are created.

After signup and verification of the same email address, the worker imports
the introduction into the owner's mail thread. Original Message-IDs and
reply IDs preserve threading and make import idempotent. Unfinished deliveries
are resolved before import. Home and the next email also attempt import immediately. Verified mail to hello then uses the normal personal
assistant and account quota, replying from hello. The old agent address remains
supported.

Unclaimed introductory content expires after 30 days. A hashed sender key and
usage metadata remain to prevent resetting the trial. Imported content belongs
to the account's normal thread history. If an account already owns the username
hello, its mailbox is preserved and the shared hello address is not advertised.
New registrations cannot claim that reserved name.
