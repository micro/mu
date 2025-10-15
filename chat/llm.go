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
Answer the question in a concise manner. Use an unbiased and compassionate tone. Do not repeat text. Don't make anything up. If you are not sure about something, just say that you don't know.

Format the response in markdown.

{{- /* Stop here if no context is provided. The rest below is for handling contexts. */ -}}
{{- if . -}}
Answer the question keeping the context of the discussion in mind. If the context is not relevant to the question, use what you know from your knowledge base but don't make anything up.

Anything below can be used as part of the conversation with the user. The bullet points are ordered by time, so the first one is the oldest.

Context:
    {{- if . -}}
    {{- range $context := .}}
    - {{.}}{{end}}
    {{- end}}
{{- end -}}
`))

type LLM struct{}

func askLLM(prompt *Prompt) (string, error) {
	m := new(Model)
	return m.Generate(prompt)
}
