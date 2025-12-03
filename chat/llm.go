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
Answer questions concisely and accurately. Be helpful and direct.

{{- if . }}

Here is some information that may be useful:
{{- range $context := . }}
- {{ . }}
{{- end }}
{{- end }}

Format responses in markdown. For brief summaries (2-3 sentences), use plain paragraph text without bullets, lists, or asterisks.
`))

type LLM struct{}

func askLLM(prompt *Prompt) (string, error) {
	m := new(Model)
	return m.Generate(prompt)
}

// AskLLM is the exported version for use by other packages
func AskLLM(prompt *Prompt) (string, error) {
	return askLLM(prompt)
}
