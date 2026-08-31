# The home

[MODEL.md](MODEL.md) says what this is built from. This says what it is for,
and it does so in a metaphor, which that document distrusts on principle: *no
metaphors that do not forbid something.*

So this one forbids things. A home is not a feed, so no surface here scrolls
forever. A home is bounded — you do not add a ninth room because a ninth thing
happened — so the catalogue stays roughly the size it is. Everything in a home
has a place, so nothing belongs in a list of thirty-five. A home has more than
one resident, so any surface with exactly one author is a defect. Nobody is
notified by their kitchen, so a thing you walk past beats a thing that
interrupts. And a home is yours: no surface here is filled by somebody else's
ranking.

If a proposal violates one of those, the metaphor has done its job.

## Why a home

The premise is replacing your own use of the internet, which lands on a
personal server whether you meant it to or not. The part that is not obvious is
who it is for: not you alone, and not agents alone. A home has residents, it
has utilities, it has a letterbox onto the outside, and it has guests. Agents
are residents. That is the whole reason the framing pays for itself — it is
the only one that makes "another party lives here and does things while you are
out" an ordinary sentence rather than a feature.

## The rooms

| In a house | Here | State |
|---|---|---|
| Street address, front door | `/`, `/@name` | built |
| Letterbox | `/inbox` — mail, sms, notify | built, the strongest thing here |
| The hall you stand in | `/home` | built |
| The voice on the counter | agent box on `/home`, `/agents` | built |
| Meter cupboard — runs unwatched | news, weather, markets, transit, hazards, flights, routes | built |
| Appliances you operate | text, images, browser, web, maps, places, video, food, docs | built |
| The workbench | shell, apps, files, and the second built-in agent | built |
| Loft, filing cabinet | archive, recall, stream | built |
| Living room | video, blog, social, news | built |
| Quiet room | prayer | built |
| Wall calendar, chore list | events, tasks | built |
| Meter and bills | wallet, usage, account | built |
| Locks and keys | auth, token, admin | built |
| Address book, who is in | contacts, users, chat, presence | built |
| **Fridge door** | notes, unrendered | **half** |
| **Spare room — a guest, in one room only** | — | **missing** |
| **The bin** | — | **missing** |
| **Doorbell wiring — things that ring each other** | events firing an agent prompt | **thread of it** |

The house is close to complete. There is a prayer room and a train timetable.
What is thin is everything a second person would touch.

## The four gaps

**No rooms.** Thirty-five services, flat, behind a nav item called Services.
That is a hardware shop. Nobody keeps thirty-five appliances in one room. The
grouping wanted is about six: what arrived, what is outside, what I keep, what
I make, what I watch, what it costs.

**The house never speaks first.** Nearly every path is a person reaching for a
thing. News fetching and a fired event are the only two motions that originate
here. A voice assistant you have to press is a calculator.

**Every surface has one author.** `notes` is per-owner and tool-facing; there
is no object in this system that two residents can both see and both write. The
missing two-way dialogue is not a chat feature — it is that nothing is shared.

**Composition happens only inside a model call.** Services are leaves and stay
leaves; that rule is right and enforced. The consequence is that the only thing
able to combine two services is a model choosing to. "Your train is cancelled
and it is raining" has nowhere to live that is not a prompt. A real house
composes without anybody thinking: the thermostat reads the weather.

## The next move

The fridge door, because it is nearly built and it closes two gaps at once. It
gives the house somewhere to speak from without interrupting, and it is the
first object here with more than one author. Rooms, guests and wiring all get
easier once something shared exists to put in them.

`service/notes` is already most of it: per-owner, title-addressed, never
expires, described as "what you wrote down, and what an agent wrote down for
you", and written by agents through `notes_add` today. It has no `Card`, so it
renders on no wall. What it is missing is a surface, a scope wider than one
person, and a way for something to fall off.

## Open

The workbench agent is called Code, which is a name that invites a comparison
it does not need and describes less than it does. Work, Build, Workshop — not
settled. Related but separate: whether `/inbox` should be `/work` at all. It
should not be both.
