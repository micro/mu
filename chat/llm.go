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
You are a helpful assistant. Answer questions concisely and accurately using the provided context.

{{- if . }}

Use the information below to answer the user's question. If the context contains relevant data like prices, dates, or specific facts, use those exact values in your answer. If the context doesn't contain the information needed, say so clearly.

Context:
{{- range $context := . }}
- {{ . }}
{{- end }}
{{- end }}

Format your response in markdown. Be direct and factual.
`))

type LLM struct{}

func askLLM(prompt *Prompt) (string, error) {
	m := new(Model)
	return m.Generate(prompt)
}
