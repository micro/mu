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

CRITICAL: Real-time data has been provided below. When asked about prices, stocks, crypto, or market data, YOU MUST use the prices shown in CURRENT MARKET PRICES. Do NOT say you cannot access real-time data - the data is provided to you below.

{{- if gt (len .Rag) 0 }}
The first source marked [PRIMARY TOPIC] contains the most relevant information for this query.
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
1. For price queries: Extract and report the exact price from CURRENT MARKET PRICES above. Example: "BTC is currently $97,485.59"
2. Use the context sources for all factual information
3. Never say "I don't have access to real-time prices" - the prices ARE provided above
4. For topics not in the sources, use your general knowledge

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
