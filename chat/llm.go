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
You are a helpful AI assistant.{{if .Topic}} The user has selected the "{{.Topic}}" topic for this conversation.{{end}}

{{- if .Rag }}

Context sources are provided below. Use them when they contain relevant information, but you can also use your general knowledge to answer questions.

{{- if gt (len .Rag) 0 }}
IMPORTANT: The first source listed below is the PRIMARY TOPIC of discussion when applicable.
{{- end }}

Context sources:
{{- range $index, $context := .Rag }}
{{- if eq $index 0 }}
[PRIMARY TOPIC] {{ . }}
{{- else }}
[Source {{ $index }}] {{ . }}
{{- end }}
{{- end }}

Instructions:
1. If the context sources contain specific information relevant to the question, use that information
2. For factual questions about specific people, events, or details mentioned in the sources, rely primarily on the sources
3. For open-ended questions, general advice, or topics not covered in the sources, you can use your general knowledge
4. If asked about specific details that should be in the sources but aren't found, say "The provided sources don't contain that specific information"
5. Always provide helpful, informative answers when you have the knowledge to do so

{{- else }}

No specific context sources provided. Use your general knowledge to provide helpful answers.
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
