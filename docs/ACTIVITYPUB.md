# ActivityPub Federation

Mu supports [ActivityPub](https://activitypub.rocks/), the open social federation protocol used by Mastodon, Threads, WordPress, and other platforms. This allows users on other networks to discover and view Mu blog posts.

## Overview

ActivityPub federation is read-only. Remote users can discover Mu profiles and view public blog posts from any ActivityPub-compatible client. No additional dependencies or services are required.

## Configuration

Set your instance domain so ActivityPub URLs resolve correctly:

```bash
export MU_DOMAIN="yourdomain.com"
```

Falls back to `MAIL_DOMAIN`, then `localhost` if not set.

## Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/.well-known/webfinger` | GET | User discovery (`acct:user@domain`) |
| `/@{username}` | GET | Actor profile (with `Accept: application/activity+json`) |
| `/@{username}/outbox` | GET | User's public blog posts as OrderedCollection |
| `/@{username}/inbox` | POST | Incoming activity stub |
| `/post?id={id}` | GET | Single post as Note (with `Accept: application/activity+json`) |

## How It Works

### User Discovery (WebFinger)

Other servers look up users via WebFinger:

```bash
curl "https://yourdomain.com/.well-known/webfinger?resource=acct:alice@yourdomain.com"
```

Returns a link to the user's ActivityPub actor profile.

### Actor Profile

When a Mastodon user searches for `@alice@yourdomain.com`, their server fetches the actor profile:

```bash
curl -H "Accept: application/activity+json" "https://yourdomain.com/@alice"
```

Returns a JSON-LD Person object with inbox and outbox URLs.

### Outbox (Blog Posts)

A user's public blog posts are served as an ActivityPub OrderedCollection:

```bash
curl -H "Accept: application/activity+json" "https://yourdomain.com/@alice/outbox"
```

Each post is represented as an ActivityPub Note with rendered HTML content, hashtags, and public addressing.

### Individual Posts

Single posts can be fetched as ActivityPub objects:

```bash
curl -H "Accept: application/activity+json" "https://yourdomain.com/post?id=123"
```

### Content Negotiation

The same URLs serve both HTML (for browsers) and ActivityPub JSON-LD (for federation). The response format is determined by the `Accept` header:

- `Accept: text/html` → HTML page
- `Accept: application/activity+json` → ActivityPub JSON-LD
- `Accept: application/ld+json` → ActivityPub JSON-LD

### Inbox

The inbox endpoint accepts POST requests for incoming activities. This is a stub for future expansion — messages are acknowledged but not yet processed.

## Limitations

- **Read-only** — Remote users can view posts but interactions (follow, like, reply) are not yet processed
- **No HTTP signatures** — Incoming messages are not cryptographically verified yet
- **No key pairs** — Actor profiles don't include public keys for signature verification

These are planned for future releases as federation matures.
