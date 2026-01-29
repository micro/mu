// Package ai provides LLM integration for the Mu platform.
// It supports multiple providers: Anthropic Claude, Fanar, and Ollama.
package ai

import (
	"strings"
	"text/template"
)

// Priority levels for LLM requests
const (
	PriorityHigh   = 0 // User-facing chat
	PriorityMedium = 1 // Headlines, topic summaries
	PriorityLow    = 2 // Background article summaries, auto-tagging
)

// Message represents a conversation message
type Message struct {
	Prompt string
	Answer string
}

// History is a list of conversation messages
type History []Message

// Provider constants
const (
	ProviderDefault   = ""          // Use configured default
	ProviderAnthropic = "anthropic" // Force Anthropic Claude
	ProviderFanar     = "fanar"     // Force Fanar
	ProviderOllama    = "ollama"    // Force Ollama
)

// Prompt represents a request to the LLM
type Prompt struct {
	System   string   // System prompt override
	Topic    string   // User-selected topic/context
	Rag      []string // RAG context sources
	Context  History  // Conversation history
	Question string   // User's question
	Priority int      // Request priority (0=high, 1=medium, 2=low)
	Provider string   // Force specific provider (empty = default)
}

// Default system prompt template
var systemPrompt = template.Must(template.New("system_prompt").Parse(`
You are a knowledgeable assistant helping with research and discussion. You have broad expertise across finance, technology, geopolitics, economics, and current events.{{if .Topic}} The conversation is focused on "{{.Topic}}".{{end}}

{{- if .Rag }}

Recent context (from news, articles, or data):
{{- range $index, $context := .Rag }}
[{{ $index }}] {{ . }}
{{- end }}

{{- end }}

How to respond:
- Use the context above as a starting point, but draw on your broader knowledge to provide depth
- Connect topics across domains (e.g., how monetary policy affects crypto, how geopolitics affects markets)
- For prices or real-time data: use what's provided in context, or note when data might be outdated
- Be direct and substantive - the user wants insight, not hedging
- When you don't know something current, say so and explain what you do know

Keep responses concise but informative. Use markdown for structure when helpful.
`))

// BuildSystemPrompt generates the system prompt from a Prompt struct
func BuildSystemPrompt(p *Prompt) (string, error) {
	if p.System != "" {
		return p.System, nil
	}
	sb := &strings.Builder{}
	if err := systemPrompt.Execute(sb, p); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// Ask sends a prompt to the LLM and returns the response
func Ask(prompt *Prompt) (string, error) {
	return generate(prompt)
}
