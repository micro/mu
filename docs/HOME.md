# One conversation

Micro is a personal AI agent. Mu is the runtime that provides its services,
tools, storage and channels.

The primary experience is the conversation at `/`. A visitor can ask a question,
sign in and continue the same conversation. Sign-in must not introduce a
dashboard or require the person to choose an agent before asking.

The sidebar holds conversations and focused utilities. Mobile navigation has
four destinations: Home, Inbox, Work and Services. Home means the conversation.
Inbox is communication requiring attention; Work records actionable tasks,
status, results and diagnostic evidence. Agent management is a secondary
destination.

Services remain independently useful. Mail is a mailbox; Video plays videos;
News offers articles. The agent can use their tools and present relevant
results directly in conversation. Save uses the existing Bookmarks service.
The optional feed belongs under Services, alongside its grid.

Use shared visual types for cards, lists, tables and forms. Specialized
components such as maps and media may add their own behavior. Do not restore
page-specific dashboard renderers, duplicate conversation surfaces or ambient
provider calls on the conversation page.
