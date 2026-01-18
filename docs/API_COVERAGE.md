# API Reference

This document covers the REST API for programmatic access to Mu.

**Live API docs:** [mu.xyz/api](https://mu.xyz/api) — interactive documentation with examples

## ✅ Fully Covered Features

### Authentication & Tokens
- ✅ **Create PAT Token** - `POST /token`
- ✅ **List PAT Tokens** - `GET /token`
- ✅ **Delete PAT Token** - `DELETE /token?id={id}`
- ✅ **Session Status** - `GET /session`

### Blog Posts
- ✅ **List Posts** - `GET /blog` (returns JSON with Accept: application/json)
- ✅ **Get Post** - `GET /post?id={id}` (returns JSON with Accept: application/json)
- ✅ **Create Post** - `POST /post`
- ✅ **Update Post** - `PATCH /post?id={id}`
- ✅ **Delete Post** - `DELETE /post?id={id}`
- ✅ **Add Comment** - `POST /post/{id}/comment`

### Chat/AI
- ✅ **Chat** - `POST /chat`

### News
- ✅ **Get News Feed** - `GET /news` (returns JSON with Accept: application/json)
- ✅ **Search News** - `POST /news`
- ✅ **Get Article** - `GET /news?id={id}` (returns JSON with Accept: application/json)

### Video
- ✅ **Get Latest Videos** - `GET /video` (returns JSON with Accept: application/json)
- ✅ **Search Videos** - `POST /video`

### Notes
- ✅ **List Notes** - `GET /notes` (returns JSON with Accept: application/json)
- ✅ **Get Note** - `GET /notes/{id}` (returns JSON with Accept: application/json)
- ✅ **Create Note** - `POST /notes/new`
- ✅ **Update Note** - `POST /notes/{id}`
- ✅ **Delete Note** - `GET /notes/{id}/delete`
- ✅ **Archive Note** - `GET /notes/{id}/archive`
- ✅ **Pin Note** - `GET /notes/{id}/pin`

### User Profiles
- ✅ **Get User Profile** - `GET /@{username}`
- ✅ **Update Status** - `POST /@{username}` (status field)

### Search
- ✅ **Vector Search** - Available via `/search?q={query}` (existing functionality)

## 📝 Features Available via Web Only (Not JSON API)

### Mail System
The mail system is primarily web-based and doesn't expose JSON APIs. This is intentional as mail is designed for web UI interaction. However, the underlying functions are available:

- `SendMessage()` - Internal function to send mail
- `GetMessage()` - Internal function to get a message
- `MarkAsRead()` - Internal function to mark as read
- `DeleteMessage()` - Internal function to delete
- `GetUnreadCount()` - Internal function for unread count

**Recommendation**: If mail API access is needed, consider adding dedicated endpoints.

### Admin Functions
Admin functions are web-only for security:
- User management - `/admin`
- Moderation queue - `/admin/moderate`
- Blocklist management - `/admin/blocklist`
- Flag management - `/flag`

**Note**: These are intentionally web-only to prevent abuse and require careful UI interaction.

### Home Cards
- Home page cards - Configured via `home/cards.json`, displayed at `/home`
- No API needed - it's a static configuration

## 🔐 Authentication Methods

All API endpoints support three authentication methods:

1. **Session Cookie** (Web login)
   - Obtained via `/login`
   - Stored in cookie: `session`

2. **PAT Token via Authorization Header** (Recommended for API)
   ```bash
   Authorization: Bearer YOUR_TOKEN
   ```

3. **PAT Token via X-Micro-Token Header** (Legacy)
   ```bash
   X-Micro-Token: YOUR_TOKEN
   ```

## 📋 Complete API Endpoint List

### Token Management
| Endpoint | Method | Auth Required | Description |
|----------|--------|---------------|-------------|
| `/token` | GET | Yes (session) | List all PAT tokens |
| `/token` | POST | Yes (session) | Create new PAT token |
| `/token?id={id}` | DELETE | Yes (session) | Delete PAT token |

### Blog
| Endpoint | Method | Auth Required | Description |
|----------|--------|---------------|-------------|
| `/blog` | GET | No | Get all posts (JSON with Accept header) |
| `/post?id={id}` | GET | No* | Get single post (JSON with Accept header) |
| `/post` | POST | Yes | Create new post |
| `/post?id={id}` | PATCH | Yes | Update post (author only) |
| `/post?id={id}` | DELETE | Yes | Delete post (author only) |
| `/post/{id}/comment` | POST | Yes | Add comment to post |

*Private posts require admin authentication

### Chat
| Endpoint | Method | Auth Required | Description |
|----------|--------|---------------|-------------|
| `/chat` | POST | No* | Chat with AI |

*Authentication required for full features

### News
| Endpoint | Method | Auth Required | Description |
|----------|--------|---------------|-------------|
| `/news` | GET | No | Get news feed (JSON with Accept header) |
| `/news` | POST | Yes | Search news articles |
| `/news?id={id}` | GET | No | Get specific article |

### Video
| Endpoint | Method | Auth Required | Description |
|----------|--------|---------------|-------------|
| `/video` | GET | No | Get latest videos (JSON with Accept header) |
| `/video` | POST | Yes | Search videos |

### User
| Endpoint | Method | Auth Required | Description |
|----------|--------|---------------|-------------|
| `/@{username}` | GET | No* | Get user profile and posts |
| `/@{username}` | POST | Yes (self) | Update status message |

*Private content requires admin authentication

### Search
| Endpoint | Method | Auth Required | Description |
|----------|--------|---------------|-------------|
| `/search?q={query}` | GET | No | Vector search across all content |

### Authentication
| Endpoint | Method | Auth Required | Description |
|----------|--------|---------------|-------------|
| `/session` | GET | No | Check session status |
| `/login` | POST | No | Web login (returns session cookie) |
| `/logout` | POST | Yes | Logout / invalidate session |
| `/signup` | POST | No | Create new account |
| `/account` | GET/POST | Yes | View/update account settings |

## 🎯 Content Type Support

Most endpoints support both HTML and JSON responses:

- **HTML Response**: Default when accessed via browser
- **JSON Response**: Include `Accept: application/json` header or `Content-Type: application/json`

Example:
```bash
# Get JSON response
curl -H "Accept: application/json" https://your-domain.com/blog

# Post JSON request
curl -X POST https://your-domain.com/post \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"title":"Test","content":"Content here with enough characters"}'
```

## 🚀 Recommended Additions

If you need mail API access, consider adding:

```go
// Suggested mail endpoints
GET  /mail           - List inbox messages
GET  /mail?id={id}   - Get specific message
POST /mail           - Send message
POST /mail/{id}/read - Mark as read
DELETE /mail?id={id} - Delete message
```

These can be easily added by creating JSON handlers in the mail package similar to how blog posts work.

## 📖 Documentation

Full interactive API documentation with examples is available at:
```
https://your-domain.com/api
```

This documentation is automatically generated from the endpoint definitions in `api/api.go` and includes:
- Authentication instructions
- All endpoints with parameters
- Request/response formats
- Example usage

## ✨ Summary

**All major features have API access:**
- ✅ Blog posts (full CRUD)
- ✅ Comments
- ✅ Chat/AI
- ✅ News (read + search)
- ✅ Videos (read + search)
- ✅ User profiles
- ✅ Vector search
- ✅ PAT token management
- ✅ Authentication

**Intentionally web-only:**
- Mail system (can be added if needed)
- Admin functions (security)
- Home page configuration (static)

All API endpoints are fully functional and can be used for automation with PAT tokens!
