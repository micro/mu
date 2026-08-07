# The surface, by service

`tools/list` is authoritative — an instance may run a subset, and tools are
derived from the services it has registered. This is the map, not the inventory.

Names are always `service_method`. A method returning the current set of
something is always `List`. If you can guess the service, you can guess the tool.

## Yours — needs an account

These hold one person's data. An anonymous caller is refused.

**mail** — a real SMTP server with DKIM, not a wrapper.
`mail_address(tag)` returns the address to hand out; with a tag it is a
plus-address (`owner+tag@instance`) that lands in the same inbox marked for that
tag. `mail_inbox(tag)` reads it back, filtered to that tag if given.
`mail_search(query, limit)`. `mail_send(to, subject, body)` is account-only
whatever it costs — an unaccountable caller must not be able to spend the
instance's domain reputation.

**contacts** — the address book, so a name resolves to an address.
`contacts_find(query)` before `mail_send` rather than asking the user to type an
address they have already told you once. `contacts_add(name, email, phone, note)`,
`contacts_list`, `contacts_delete(id)`.

**events** — the calendar. `events_create(title, when, note, minutes, repeat,
prompt)`, `events_list`, `events_delete(id)`, and `events_free(from, to,
minutes, day_start, day_end)` which returns open slots rather than making you
compute them from a list. `when` is an instant; send RFC3339 and be explicit
about the timezone.

An account may have connected Google Calendar, in which case `events_free` and
`events_list` include external busy periods. You cannot tell from the tool
whether it did, and you do not need to.

**tasks** — work, optionally assigned to the instance's own agent.
`tasks_create(title, detail, due, assignee)` — set `assignee` to the agent and it
picks the task up. `tasks_next` returns what to do next, `tasks_update(id,
status, result, ...)`, `tasks_list(status)`, `tasks_delete(id)`.

**files** — bytes with a URL. `files_put(name, content, type, encoding)`,
`files_get(id)`, `files_list`, `files_share(id, public)`, `files_delete(id)`.

**db** — per-account records in named collections. `db_create(collection, data,
id, public)`, `db_get`, `db_list(collection, where, sort, order, limit, scope)`,
`db_delete`. This is the durable scratchpad; see `recipes.md`.

**index** — `index_search(query, limit)` searches across the caller's own
content: their posts, notes, saved things. Reach for it before `web_search` when
the answer might already be theirs.

**wallet** — `wallet_balance`, `wallet_check(operation)`, `wallet_charge(operation)`.

**saved / content** — `saved_list`, and `content_save`, `content_unsave`,
`content_hide`, `content_flag`, each taking `(type, id)`.

## The world — works anonymously

**news** — `news_list(topic, limit)`, `news_search(query)`, `news_read(id)`.
An aggregator this instance runs, not a news API. Free.

**markets** — `markets_list(category)`: crypto, stocks, futures, commodities,
currencies.

**weather** — `weather_forecast(lat, lon)`. Takes coordinates, not a place name;
`places_geocode(address)` converts.

**web** — `web_search(query)` and `web_fetch(url)`. The open web, and the
expensive door. `web_fetch` runs a readability pass and returns text.

**places** — `places_search(query, near, radius)`, `places_nearby(address, lat,
lon, radius)`, `places_geocode(address)`, `places_eta(from, to, mode)`.

**video** — `video_list`, `video_search(query)`.

**images** — `images_search(query)` and `images_generate(prompt)`.

**prayer / quran** — `prayer_times(lat, lon, tz)`, `prayer_qibla(lat, lon)`,
`prayer_verse(chapter, verse)`, `prayer_saying(book)`, `prayer_reflection`,
`quran_search(query)`.

## Published — reading is open, writing is yours

**blog** — `blog_list`, `blog_read(id | title)`, `blog_create(title, content)`,
`blog_update(id, ...)`, `blog_delete(id | title)`. Read and delete take a title
as well as an id, resolved exact → unique prefix → unique substring. An
ambiguous title returns nothing rather than guessing, which matters most for
delete.

**apps** — small hosted programs. `apps_search(query, tag)`, `apps_read(slug)`,
`apps_create(name, slug, description, html, price, tags)`, `apps_edit`,
`apps_fork(slug, new_slug)`, `apps_test(slug)`, `apps_build(prompt)` to generate
one, `apps_run(code)` for a throwaway sandbox. Apps run in the browser; there is
no server-side execution.

**stream** — `stream_list`, `stream_post(content)`. This instance's own event
timeline.

**social** — `social_list`, `social_search(query)`. Public discussion threads.

**chat** — `chat_rooms(limit)`, `chat_messages(room)`. Live discussion rooms
attached to an item. Read-only by design: a room is a websocket conversation
between people who are present, and a message injected from outside would appear
from nobody.

## The instance's own model

**agent_ask(prompt)** plans and calls tools on the instance side, and returns a
written answer. Delegate a whole task with it.

It spends a model call, so for a single fact call the tool directly. There is no
cheap "ask the index a question" tool — searching the index *is* that, at
`index_search`, and it costs nothing.

## Moderation

`block_user(user)`, `unblock_user(user)`.
