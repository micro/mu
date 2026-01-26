# Mu SDK

The Mu SDK is automatically available in all micro apps as `window.mu`.

> **Note:** For programmatic access outside of micro apps, see the [REST API](/docs/api).

## Database (mu.db)

Per-user persistent storage. 100KB quota per app.

```javascript
// Get a value
const value = await mu.db.get('key');

// Set a value (can be any JSON-serializable data)
await mu.db.set('key', value);

// Delete a key
await mu.db.delete('key');

// List all keys
const keys = await mu.db.list();

// Check quota
const {used, limit} = await mu.db.quota();
```

## Fetch (mu.fetch)

Server-side proxy for fetching external URLs. Bypasses CORS restrictions.

```javascript
// Fetch any URL (no CORS issues!)
const response = await mu.fetch('https://api.example.com/data');
if (response.ok) {
  const text = await response.text();
  const json = await response.json();
}
```

**Always use `mu.fetch()` instead of `fetch()` for external URLs.**

## Cache (mu.cache)

Client-side caching with TTL support. Uses localStorage - data persists across page loads but not across devices.

```javascript
// Cache data with 1 hour TTL
await mu.cache.set('markets', data, { ttl: 3600 });

// Get cached value (returns null if expired or missing)
const data = await mu.cache.get('markets');

// Cache without expiration
await mu.cache.set('settings', prefs);

// Delete cached item
await mu.cache.delete('markets');

// Clear all cached items for this app
await mu.cache.clear();
```

**Use `mu.cache` for API responses and temporary data. Use `mu.db` for persistent user data that syncs across devices.**

## Theme (mu.theme)

CSS variables are automatically injected. Use them for consistent styling.

```css
/* Available CSS variables */
var(--mu-text-primary)      /* #1a1a1a */
var(--mu-text-secondary)    /* #555 */
var(--mu-text-muted)        /* #888 */
var(--mu-accent-color)      /* #0d7377 */
var(--mu-accent-blue)       /* #007bff */
var(--mu-card-background)   /* #ffffff */
var(--mu-card-border)       /* #e8e8e8 */
var(--mu-hover-background)  /* #fafafa */
var(--mu-spacing-xs/sm/md/lg/xl)
var(--mu-border-radius)     /* 6px */
var(--mu-shadow-sm)
var(--mu-shadow-md)
var(--mu-font-family)
```

```javascript
// Get value in JS
const color = mu.theme.get('accent-color');
```

## User Context (mu.user)

```javascript
mu.user.id        // User ID (string) or null if not logged in
mu.user.name      // User's display name or null
mu.user.loggedIn  // boolean
```

## App Context (mu.app)

```javascript
mu.app.id    // This app's unique ID
mu.app.name  // This app's name
```

## Example: Fetch External API

```javascript
// Fetch a webpage (mu.fetch bypasses CORS)
const url = document.getElementById('url').value;
const response = await mu.fetch(url);
if (response.ok) {
  const html = await response.text();
  document.getElementById('content').textContent = html;
}
```

## Example: Todo App with Persistence

```javascript
// Load todos on startup
const todos = await mu.db.get('todos') || [];

// Save after changes
async function addTodo(text) {
  todos.push({ id: Date.now(), text, done: false });
  await mu.db.set('todos', todos);
  render();
}
```

## Actions (mu.action)

Register callable actions that flows can invoke:

```javascript
// Register an action that flows can call
mu.action({
  name: 'get_data',
  description: 'Get processed data from this app',
  params: { filter: 'string' },
  handler: async (params) => {
    const data = await mu.db.get('data') || [];
    if (params.filter) {
      return data.filter(d => d.name.includes(params.filter));
    }
    return data;
  }
});
```

This allows flows to call your app:

```
run my-app get_data with filter "important"
then email to me
```

> **Note:** Action execution in flows is currently planned. Apps must be run interactively for now.

## Featured Apps

- [Todo](/apps/todo) - Task management
- [Timer](/apps/timer) - Focus/pomodoro timer  
- [Expenses](/apps/expenses) - Expense tracking
