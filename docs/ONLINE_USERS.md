# Online Users Feature

## Overview

The online users feature provides real-time visibility into who's currently active on the Mu platform. It displays a widget on the home page showing a count and list of users who have been active in the last 3 minutes.

## Implementation

### Components

1. **Presence Tracking** (`auth/auth.go`)
   - Automatically tracks user activity
   - Maintains a map of username → last seen timestamp
   - `UpdatePresence(username)` - Called during authentication and page loads
   - `GetOnlineUsers()` - Returns list of users active within 3 minutes

2. **Home Page Widget** (`home/home.go`)
   - `OnlineUsersCard()` - Renders the online users list
   - Shows green dot indicator for online status
   - Displays count: "X Online"
   - Lists usernames with links to /chat

3. **Configuration** (`home/cards.json`)
   ```json
   {
     "id": "online",
     "title": "Online Users",
     "type": "online",
     "position": 1,
     "link": "/chat"
   }
   ```

### Design Patterns

**Visual Design:**
- Green dot (🟢 #28a745) indicates online status
- Minimal, clean UI matching existing cards
- Responsive flexbox layout
- Consistent with Mu design system

**User Experience:**
- Shows "X Online" count at top
- Lists all online users with clickable links
- Empty state: "No users currently online"
- Links to /chat for interaction

**Caching:**
- 2-minute TTL (same as other home cards)
- Automatic refresh on page load
- Balances freshness with performance

## Usage

### For Users

**Viewing Online Users:**
1. Login to Mu
2. Navigate to home page
3. See "Online Users" card in left column
4. View count and list of active users

**Interacting:**
- Click any username to go to /chat
- Start a conversation with online users
- See who's available in real-time

### For Developers

**Adding Presence Updates:**
```go
import "mu/auth"

// Update user presence after authentication
auth.UpdatePresence(username)

// Get list of online users
onlineUsers := auth.GetOnlineUsers()
```

**Customizing Display:**
Edit `home/home.go` `OnlineUsersCard()` function to change:
- Visual styling
- Time threshold (default: 3 minutes)
- Click behavior
- Additional user info

## Configuration

### Presence Timeout

Default: Users are considered online if active within **3 minutes**

To change, edit `auth/auth.go`:
```go
func GetOnlineUsers() []string {
    // Change this duration
    if now.Sub(lastSeen) < 3*time.Minute {
        online = append(online, username)
    }
}
```

### Card Position

Edit `home/cards.json` to change widget position:
```json
{
  "left": [
    {
      "id": "online",
      "position": 1  // Change position
    }
  ]
}
```

## Technical Details

### Presence Tracking Flow

1. **User Login:**
   - `auth.Login()` → `UpdatePresence(username)`
   - Timestamp recorded in `userPresence` map

2. **Page Access:**
   - Authentication middleware updates presence
   - Keeps timestamp current during active browsing

3. **Display:**
   - `OnlineUsersCard()` calls `GetOnlineUsers()`
   - Filters users with recent timestamps
   - Returns list of active usernames

### Performance

- **Memory:** O(n) where n = number of registered users
- **Lookup:** O(1) presence updates
- **Scan:** O(n) to build online users list
- **Caching:** 2-minute TTL reduces frequent lookups

### Thread Safety

- `presenceMutex` protects concurrent access
- `sync.RWMutex` allows concurrent reads
- Write locks only for presence updates

## Future Enhancements

### Planned Features

1. **Direct Messaging Integration**
   - Click user → start DM conversation
   - Show unread message indicators
   - Real-time message notifications

2. **Enhanced Status**
   - User avatars
   - Custom status messages
   - "Do Not Disturb" mode
   - Last seen timestamps

3. **Real-time Updates**
   - WebSocket connection for live updates
   - Instant presence changes
   - No page refresh needed

4. **Advanced Filtering**
   - Search online users
   - Filter by department/role
   - Show mutual connections

5. **Privacy Controls**
   - Hide online status option
   - Invisible mode
   - Selective visibility

### API Endpoints

**Get Online Users (JSON):**
```bash
GET /api/users/online
Authorization: Bearer <token>

Response:
{
  "online": ["alice", "bob", "charlie"],
  "count": 3
}
```

**Update Own Presence:**
```bash
POST /api/presence
Authorization: Bearer <token>

Response: 204 No Content
```

## Troubleshooting

### Users Not Showing as Online

**Issue:** Active users don't appear in online list

**Solutions:**
1. Check presence timeout (default 3 minutes)
2. Verify `UpdatePresence()` is called during auth
3. Clear browser cache and login again
4. Check server logs for errors

### Performance Issues

**Issue:** Slow page loads with many users

**Solutions:**
1. Increase cache TTL in `home.go`
2. Implement pagination for large user lists
3. Add Redis for distributed presence tracking
4. Optimize `GetOnlineUsers()` query

### Widget Not Appearing

**Issue:** Online users card missing from home page

**Solutions:**
1. Verify `home/cards.json` includes "online" card
2. Check card is registered in `cardFunctions` map
3. Ensure user is authenticated
4. Rebuild application: `go build`

## Security Considerations

### Privacy

- Only authenticated users can see online status
- No external API exposure without auth
- Users cannot hide from online list (by design)
- Consider adding privacy settings in future

### Rate Limiting

- Presence updates are rate-limited by auth middleware
- Home page cache prevents excessive presence checks
- No direct API endpoint for presence updates yet

### Data Retention

- Presence data stored in memory only
- No persistent storage of online status
- Automatically expires after 3 minutes
- Server restart clears all presence data

## Related Documentation

- [Authentication System](./AUTH.md)
- [Home Page Cards](./HOME_CARDS.md)
- [Chat System](./CHAT.md)
- [Privacy & Security](./SECURITY.md)

## Credits

Feature developed as part of the Mu platform to enhance social interaction and real-time collaboration among users.
