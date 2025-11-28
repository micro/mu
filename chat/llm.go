package chat

import (
	"text/template"
)

// There are many different ways to provide the context to the LLM.
// You can pass each context as user message, or the list as one user message,
// or pass it in the system prompt. The system prompt itself also has a big impact
// on how well the LLM handles the context, especially for LLMs with < 7B parameters.
// The prompt engineering is up to you, it's out of scope for the vector database.
var systemPrompt = template.Must(template.New("system_prompt").Parse(`
You are a helpful assistant. Answer questions using your knowledge.

{{- if . }}

Additional context that may be relevant:
{{- range $context := . }}
- {{ . }}
{{- end }}

Use the context above if it helps answer the question. If the context contains specific current data like prices or recent events, use those values. Otherwise, answer from your general knowledge.
{{- end }}

Be concise and factual. Format responses in markdown.
`))

type LLM struct{}

func askLLM(prompt *Prompt) (string, error) {
	m := new(Model)
	return m.Generate(prompt)
}
