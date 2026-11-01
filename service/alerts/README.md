# Alerts

An alert is a condition that needs attention. Events schedule work; notifications
carry messages. Alerts retain the condition independently of either.

This first slice provides:

- Private account records and group records, with current membership required to
  read and group administrator permission required to change shared records.
- `List`, `Raise` and `Update` through the normal service API, MCP and CLI.
- Open, acknowledged, resolved and dismissed states. A stable source/key pair
  updates one record; repeated checks preserve acknowledgement and dismissal.
- A page at `/alerts`, an attention summary in Inbox, and the same summary in
  Home's existing Inbox card. No additional primary navigation.
- Existing Admin disk/traffic checks as producers. Recovery resolves the condition;
  a later crossing starts a new episode. Log warnings remain discrete occurrences.
  Existing mail/push delivery is attempted once per new episode, subject to the
  configured cooldown. Delivery is best-effort, not a durable retry queue.

Custom API alerts use the `custom` source and cannot overwrite system records.
Resolving a custom alert marks its condition recovered, allowing the next Raise
with that key to open it again. Dismissal suppresses repeated raises until the
producer resolves it. Only the producer can mark a monitored system condition
recovered; manually resolving it does not cause another alert on the next poll.

Records are stored atomically and fail closed on damaged storage. Storage is
limited to 1,000 conditions per owner; the oldest recovered records are discarded
at that limit, while active conditions are retained. Account deletion clears
personal records. Deleted group records are inaccessible under the membership
checks, consistent with existing shared-resource retention.

Detection and delivery policy belong in application composition, not in this
service. Raising a custom alert creates an Inbox attention record without sending
mail or push. This does not yet monitor personal mail, calendars, news or marketing,
create anomaly rules, add delivery preferences, or give group agents new tools.
Those integrations should be explicit, scoped rules with stable deduplication
keys, recovery criteria and an explanation of why the user should care.
