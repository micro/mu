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
You are a helpful AI assistant.{{if .Topic}} The user has selected the "{{.Topic}}" topic for this conversation.{{end}}

{{- if .Rag }}

CRITICAL: Real-time data has been provided below. When asked about prices, stocks, crypto, or market data, YOU MUST use the prices shown in CURRENT MARKET PRICES. Do NOT say you cannot access real-time data - the data is provided to you below.

Context sources (use these for factual information):
{{- range $index, $context := .Rag }}
[Source {{ $index }}] {{ . }}
{{- end }}

Instructions:
1. For price queries: Extract and report the exact price from CURRENT MARKET PRICES above
2. For follow-up questions with pronouns (him, her, it, they, this, etc.), refer to the conversation history to understand what the user is asking about
3. Use context sources for factual information, but prioritize conversation continuity
4. For topics not in the sources, use your general knowledge

{{- else }}

No specific context sources provided. Use your general knowledge to provide helpful answers.
{{- end }}

Format responses in markdown. For brief summaries (2-3 sentences), use plain paragraph text without bullets, lists, or asterisks.
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
