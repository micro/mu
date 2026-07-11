# The 100 everyday services

Mu's thesis: the everyday internet, handled by one agent — each capability a
real, self-hostable service, not a feature buried in a big platform. We ship
about a dozen today (news, mail, markets, weather, search, video, social, blog,
places, reminder, payments, apps, chat). This is the map of the ~100 services
people use every day — the dozen we have, and the ~88 to build next.

The existing services are the template. Each new one is the same shape:

- a **go-micro service** with a clean request/response contract,
- **agent-callable** as a tool, and reachable over web, REST, and MCP,
- **own-your-data**: per-account, scoped, self-hostable,
- built to be **useful on its own** and better when the agent composes it with
  the others.

## The one rule: reuse before you integrate

Prefer services we can stand up on what we already run — storage, auth, the AI
layer, the search corpus, the reader/web-fetch, mail/SMTP, the scheduler, and our
existing data services (markets, news, weather, places). Add a **new external
source only when the capability genuinely can't exist without it**, and mark it
so the cost is explicit. Most daily services are just *your data + AI + storage*
— which is exactly the kind of thing the big platforms bundle and we can offer
open.

Legend: **✓** shipped · **○** to build · `(reuse: …)` existing infra it rides on ·
`(ext: …)` a new external source it would require.

## Communication (1–10)
1. **mail** ✓
2. **chat / messaging** ✓
3. **contacts / address book** ○ (reuse: storage, auth)
4. **calendar** ○ (reuse: storage, reminder)
5. **scheduling / availability booking** ○ (reuse: calendar, mail)
6. **newsletters / broadcasts** ○ (reuse: mail, blog)
7. **notifications inbox** ○ (reuse: event bus, channels)
8. **voice notes / dictation** ○ (reuse: AI; ext: speech-to-text)
9. **video / voice rooms** ○ (ext: WebRTC/SFU)
10. **groups / community spaces** ○ (reuse: social, chat)

## Notes, docs & files (11–20)
11. **notes** ○ (reuse: storage, AI)
12. **tasks / to-do** ○ (reuse: storage)
13. **projects / kanban** ○ (reuse: tasks)
14. **documents / writing** ○ (reuse: storage, AI)
15. **spreadsheets / tables** ○ (reuse: storage)
16. **files / drive** ○ (reuse: storage)
17. **bookmarks** ○ (reuse: web-fetch, corpus)
18. **read-later / reader** ○ (reuse: reader/web-fetch)
19. **password vault** ○ (reuse: encrypted storage)
20. **forms & surveys** ○ (reuse: storage)

## Money & finance (21–30)
21. **payments / wallet** ✓
22. **markets** ✓
23. **budget / expenses** ○ (reuse: storage, AI)
24. **bill split** ○ (reuse: storage)
25. **invoices / billing** ○ (reuse: storage, mail)
26. **subscriptions tracker** ○ (reuse: storage, mail)
27. **receipts** ○ (reuse: storage; AI OCR)
28. **crypto portfolio** ○ (reuse: markets, wallet)
29. **price / deal tracker** ○ (reuse: web-fetch, scheduler)
30. **accounts overview / net worth** ○ (reuse: storage; ext: bank sync, optional)

## Media & entertainment (31–39)
31. **video** ✓
32. **podcasts** ○ (reuse: feed poller/reader)
33. **music / audio library** ○ (reuse: storage/files)
34. **books / reading list** ○ (reuse: corpus, storage)
35. **watchlist (film/TV)** ○ (reuse: storage; ext: title catalog)
36. **events & tickets** ○ (reuse: corpus; ext: listings, optional)
37. **sports scores & standings** ○ (ext: scores feed)
38. **games & puzzles** ○ (reuse: apps)
39. **quizzes / trivia** ○ (reuse: AI, corpus)

## Information & knowledge (40–49)
40. **search** ✓
41. **news** ✓
42. **assistant / Q&A** ✓
43. **wiki / knowledge base** ○ (reuse: corpus, storage)
44. **translate** ○ (reuse: AI)
45. **dictionary / thesaurus** ○ (reuse: AI, corpus)
46. **summarize / TL;DR** ○ (reuse: AI, web-fetch)
47. **transcribe** ○ (ext/AI: speech-to-text)
48. **flashcards / study** ○ (reuse: AI, storage)
49. **how-to / recipes** ○ (reuse: corpus, AI)

## Travel & local (50–58)
50. **places / maps** ✓
51. **weather** ✓
52. **directions / transit** ○ (ext: routing)
53. **flights** ○ (ext: flight data)
54. **trips / itinerary** ○ (reuse: AI, calendar, storage)
55. **package tracking** ○ (ext: carrier)
56. **phrasebook** ○ (reuse: AI, translate)
57. **currency & tips** ○ (reuse: markets)
58. **local recommendations** ○ (reuse: places, corpus, AI)

## Health & wellness (59–67)
59. **habits / streaks** ○ (reuse: storage, apps)
60. **journal / diary** ○ (reuse: storage, AI)
61. **mood / wellbeing** ○ (reuse: storage, AI)
62. **fitness / workouts** ○ (reuse: storage)
63. **nutrition / calories** ○ (reuse: storage, AI)
64. **medications** ○ (reuse: reminder)
65. **sleep log** ○ (reuse: storage)
66. **meditation / breathing** ○ (reuse: storage)
67. **symptom / first-aid guide** ○ (reuse: corpus, AI)

## Home & life admin (68–76)
68. **shopping / groceries** ○ (reuse: apps, storage)
69. **meal planner** ○ (reuse: AI, recipes)
70. **pantry / inventory** ○ (reuse: storage)
71. **chores / household** ○ (reuse: tasks)
72. **wishlists / gifts** ○ (reuse: storage, AI)
73. **important documents** ○ (reuse: files)
74. **warranties & manuals** ○ (reuse: files, corpus)
75. **vehicle / maintenance** ○ (reuse: storage, reminder)
76. **pets** ○ (reuse: storage, reminder)

## Work & dev (77–85)
77. **code snippets** ○ (reuse: storage)
78. **repos / issues** ○ (reuse: existing GitHub integration)
79. **uptime / monitoring** ○ (reuse: web-fetch, scheduler)
80. **status page** ○ (reuse: monitoring)
81. **feedback / support desk** ○ (reuse: mail, storage)
82. **CRM / pipeline** ○ (reuse: contacts, storage)
83. **time tracking** ○ (reuse: storage)
84. **meeting notes** ○ (reuse: AI, transcribe)
85. **standups / updates** ○ (reuse: storage, social)

## Utilities & tools (86–95)
86. **reminders** ✓
87. **calculator** ○ (reuse: AI)
88. **unit converter** ○ (reuse: markets for currency)
89. **timers / alarms / stopwatch** ○ (reuse: scheduler)
90. **world clock / time zones** ○ (self-contained)
91. **QR / barcode** ○ (self-contained)
92. **URL shortener** ○ (reuse: storage)
93. **PDF / document tools** ○ (self-contained)
94. **image tools** ○ (self-contained)
95. **text-to-speech** ○ (ext/AI)

## Personal & social (96–100)
96. **social** ✓
97. **blog** ✓
98. **profile / link-in-bio** ○ (reuse: blog, storage)
99. **resume / CV** ○ (reuse: storage, AI)
100. **daily brief / on-this-day** ○ (reuse: corpus, calendar, digest)

## How this gets built

- **Reuse-heavy first.** The services marked `(reuse: storage/AI/corpus/…)` need
  no new external source — they are the fastest to ship and the truest to the
  "own your services" pitch. Do these before anything marked `(ext: …)`.
- **Agent-composed by default.** Each service is a tool the agent can call, so
  value compounds: calendar + mail + contacts → scheduling; recipes + pantry +
  shopping → meal planning; markets + wallet → crypto portfolio.
- **A shipped service is:** the go-micro service, a web surface, REST + MCP
  exposure, per-account data, and an agent tool — the same bar the current dozen
  already meet.
