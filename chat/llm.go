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
You are a helpful assistant. Answer questions concisely and accurately.

{{- if . }}

The following context may contain relevant information:

Context:
{{- range $context := . }}
- {{ . }}
{{- end }}

Use the context above if it's relevant, but also use your own knowledge to provide accurate answers. If specific data like prices or dates are in the context, prioritize those exact values.
{{- else }}

Answer based on your knowledge.
{{- end }}

Format your response in markdown. Be direct and factual.
`))

type LLM struct{}

func askLLM(prompt *Prompt) (string, error) {
	m := new(Model)
	return m.Generate(prompt)
}
