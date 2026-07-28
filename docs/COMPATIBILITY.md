# Compatibility promise

Mu follows [semantic versioning](https://semver.org). From 1.0.0 onward this
document defines what that actually guarantees — which surfaces you can build
on, which you can't, and what happens to your data when you upgrade.

The short version: **if you self-host Mu, upgrading should never lose your data
or break your integrations.** Everything below is the detail behind that
sentence.

## What is stable

These are public surfaces. Within a major version they do not break: nothing is
removed or given incompatible behaviour, and additions are backwards compatible.

| Surface | Guarantee |
|---|---|
| **MCP tools** (`/mcp`) | Tool names and their input fields keep working. A renamed tool keeps its old name as an alias. New optional fields may be added. |
| **REST API** | Existing paths and response fields keep working. New fields may be added — parse leniently and ignore unknown ones. |
| **CLI** (`mu <command>`) | Existing subcommands and flags keep working. Output is human-readable and may be reformatted; script against the API or `--json` where available, not against prose. |
| **Environment variables** | Names and meanings are stable. Defaults may change only where the old default was a bug (a security or data-loss fix); such changes are called out in the release notes. |
| **On-disk data** | See [Your data](#your-data) below. |
| **A2A** (`/a2a`) | Stable as an endpoint. See [Known exceptions](#known-exceptions). |

## What is not stable

Everything else. Specifically, do not build on:

- **Go packages.** `internal/...` and the domain packages are implementation
  detail. Mu is distributed as a binary, not a library. Import at your own risk.
- **go-micro service method signatures.** The RPC handlers behind each service
  (`weather.Server.Forecast` and friends) are reachable in-process and may change
  shape; the MCP and REST surfaces in front of them are what's stable.
- **HTML, CSS, and DOM structure.** The web UI is redesigned freely. Scrape at
  your own risk; use the API.
- **Card internals and home-screen layout.** Which cards exist, their order and
  their markup all change.
- **AI output.** Model responses are inherently non-deterministic and providers
  change. Prompt wording, tool-selection behaviour and answer formatting are not
  contractual.
- **Anything documented as experimental** or gated behind an off-by-default
  environment flag.

## Your data

Mu keeps your data on your disk. The promise:

1. **Upgrades never require a manual migration.** A newer Mu reads an older
   Mu's data directory as-is.
2. **Fields are added, not removed or repurposed.** New fields are optional and
   absent values mean "not set", so an older file is always valid input to a
   newer binary. Where a new field needs a different default for existing users,
   the code detects the legacy shape explicitly rather than guessing.
3. **Writes are atomic.** Every store write goes to a temporary file, is
   fsynced, then renamed into place. A crash, a power cut or a full disk leaves
   the previous contents intact — never a half-written file.
4. **Credentials are owner-only.** Everything under the data directory is
   written `0600` inside a `0700` directory.

**Downgrades are not supported.** An older binary may not understand fields a
newer one wrote. Back up before upgrading if you intend to be able to roll back.

**Back up by copying the data directory** (`~/.mu` by default) while the server
is stopped. There is no built-in backup command yet.

## Deprecation

When something stable has to change:

1. The old form keeps working, and its replacement ships alongside it.
2. The old form is documented as deprecated in the release notes.
3. It is removed no earlier than the next **major** version.

MCP tool renames are the worked example: `markets` became `markets_list`, and
`markets` still resolves. Likewise `web_search`/`web_fetch` are now
`search_web`/`search_fetch`, `image_generate`/`image_search` are
`images_generate`/`images_search`, and `reminder` is `islam` — every old name
still works. That is the pattern for every rename.

## Known exceptions

Called out honestly, because a promise with hidden holes is worse than no
promise:

- **`/a2a`** is a hand-rolled implementation of the A2A protocol and is intended
  to be replaced by go-micro's gateway once that lands upstream. The endpoint
  and its purpose are stable; exact payload details may shift as it converges on
  the spec.
- **Search index storage** (`MU_USE_SQLITE`) has two backends. Both read the
  same data through the same API; the flag chooses the engine, not the schema.
- **AI provider behaviour** changes outside our control, as above.

## Version scheme

- **Patch** (1.0.x) — fixes only.
- **Minor** (1.x.0) — new services, tools, and features; always backwards
  compatible.
- **Major** (x.0.0) — reserved for breaking changes to a stable surface, with
  migration notes.

`mu version` reports the release; `GET /version` reports it as `version`
alongside build and dependency detail.
