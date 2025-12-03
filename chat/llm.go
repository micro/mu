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
You are a helpful AI assistant.{{if .Topic}} The user has selected the "{{.Topic}}" topic for this conversation.{{end}} Answer questions accurately based ONLY on the provided context sources.

{{- if .Rag }}

IMPORTANT: The first source listed below is the PRIMARY TOPIC of discussion. Your answer MUST be directly related to this topic.

Context sources:
{{- range $index, $context := .Rag }}
{{- if eq $index 0 }}
[PRIMARY TOPIC] {{ . }}
{{- else }}
[Source {{ $index }}] {{ . }}
{{- end }}
{{- end }}

Instructions:
1. Read the PRIMARY TOPIC carefully - this is what the user is asking about
2. Answer the question using information from the sources above
3. If the question asks about specifics (like "where did X move?"), search the sources for that specific information
4. If the sources don't contain the answer, say "I don't have enough information in the provided sources to answer that question"
5. NEVER make up information or answer about unrelated topics

{{- else }}

No context sources provided.
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
