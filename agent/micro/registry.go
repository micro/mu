package micro

// The agent this instance provides.
//
// One. There were eleven — News, Markets, Weather, Places, Video, Social, Mail,
// Apps, Faith, Search and this one — each with its own system prompt and its
// own slice of the tool list, reachable at agent+news@ and routed to by
// keyword.
//
// They were ten tools wearing coats. The distinction between "the news agent"
// and "Micro with the news tools" is a distinction the caller cannot use: you
// do not know in advance which specialist your question belongs to, and if you
// did you would have asked the tool. What the split actually bought was a
// router that had to guess, ten prompts to keep in step with each other, and
// ten rows on a page saying almost the same thing.
//
// Micro has every tool and no scope, which is what it always had. The other ten
// were the same tools behind a name, and the name was the only difference.
//
// The machinery stays: Register, Route and Execute are how an agent with a
// scope is run, and an account's own agents are exactly that — a name, an
// instruction, and the services it may use. What is gone is this instance
// shipping ten of them nobody asked for.
func init() {
	Register(&Agent{
		ID:           "micro",
		Name:         "Micro",
		Description:  "General-purpose personal AI — handles any query",
		SystemPrompt: `You are Micro, a personal AI assistant. You have access to all tools and can help with anything — news, markets, weather, mail, search, places, apps, and more. Be concise, direct, and helpful. Use markdown.`,
		Tools:        nil, // nil = all tools
		MemoryScope:  "",
		Examples:     []string{"Give me a morning brief", "What is moving in markets?", "Weather in San Francisco", "Find today's AI news"},
	})
}
